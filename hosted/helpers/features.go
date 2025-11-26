/*
Copyright © 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helpers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	testhelpersRancher "github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	"github.com/rancher/shepherd/clients/rancher"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// retry provides a minimal exponential backoff helper.
func retry(ctx context.Context, attempts int, base time.Duration, f func() (bool, error)) error {
	delay := base
	for i := 0; i < attempts; i++ {
		done, err := f()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			GinkgoLogr.Info(fmt.Sprintf("retry %d/%d: %v", i+1, attempts, err))
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 10*base {
			delay *= 2
		}
	}
	return errors.New("max retries exceeded")
}

// AddClusterRepo ensures a git-backed ClusterRepo exists and is ready.
func AddClusterRepo(client *rancher.Client, name, gitRepo, branch string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	GinkgoLogr.Info(fmt.Sprintf("Ensuring ClusterRepo %s (git=%s branch=%s)", name, gitRepo, branch))
	crClient := client.Catalog.ClusterRepos()

	existing, err := crClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("get clusterrepo: %w", err)
	}
	if apierrors.IsNotFound(err) {
		repo := &catalogv1.ClusterRepo{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: catalogv1.RepoSpec{
				GitRepo:   gitRepo,
				GitBranch: branch,
				Enabled:   func() *bool { b := true; return &b }(),
			},
		}
		if _, err = crClient.Create(ctx, repo, metav1.CreateOptions{}); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Created elsewhere in between: fetch and fall through to potential update
				existing, err = crClient.Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("get clusterrepo after AlreadyExists: %w", err)
				}
			} else {
				return fmt.Errorf("create clusterrepo: %w", err)
			}
		} else {
			// Successful create; refresh existing for potential readiness checks
			existing = repo
		}
	} else {
		needUpdate := false
		if existing.Spec.GitRepo != gitRepo || existing.Spec.GitBranch != branch || existing.Spec.Enabled == nil || !*existing.Spec.Enabled {
			existing.Spec.GitRepo = gitRepo
			existing.Spec.GitBranch = branch
			if existing.Spec.Enabled == nil || !*existing.Spec.Enabled {
				b := true
				existing.Spec.Enabled = &b
			}
			needUpdate = true
		}
		if needUpdate {
			if _, err = crClient.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update clusterrepo: %w", err)
			}
		}
	}

	// readiness poll
	err = retry(ctx, 12, 5*time.Second, func() (bool, error) {
		latest, e := crClient.Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		if latest.Status.Commit != "" {
			return true, nil
		}
		for _, c := range latest.Status.Conditions {
			if string(c.Type) == "Downloaded" && c.Status == "True" {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("clusterrepo %s not ready: %w", name, err)
	}
	GinkgoLogr.Info(fmt.Sprintf("ClusterRepo %s ready", name))
	return nil
}

// EnableProvider sets a Rancher feature flag to true and verifies it.
func EnableProvider(client *rancher.Client, providerName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	GinkgoLogr.Info(fmt.Sprintf("Ensuring provider %s is enabled (feature fallback)", providerName))

	feature, err := client.Management.Feature.ByID(providerName)
	if err != nil {
		// Rancher may not expose a feature for this provider (e.g., alibaba). Treat as non-fatal.
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			GinkgoLogr.Info(fmt.Sprintf("Feature '%s' not found; skipping explicit enable", providerName))
			return nil
		}
		return fmt.Errorf("get feature %s: %w", providerName, err)
	}
	if feature.Value != nil && *feature.Value {
		GinkgoLogr.Info("Feature already enabled")
		return nil
	}
	trueVal := true
	feature.Value = &trueVal
	if _, err = client.Management.Feature.Update(feature, feature); err != nil {
		return fmt.Errorf("update feature %s: %w", providerName, err)
	}
	err = retry(ctx, 10, 3*time.Second, func() (bool, error) {
		latest, e := client.Management.Feature.ByID(providerName)
		if e != nil {
			if strings.Contains(strings.ToLower(e.Error()), "not found") {
				// Feature disappeared; treat as enabled/ignored.
				return true, nil
			}
			return false, e
		}
		if latest.Value != nil && *latest.Value {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("feature %s not enabled: %w", providerName, err)
	}
	GinkgoLogr.Info(fmt.Sprintf("Feature %s enabled", providerName))
	return nil
}

// WaitForAlibabaProviderActivation waits until the Alibaba UI extension and operator pods are Ready.
// Assumes charts have already been installed (ali-ui repo + AlibabaCloud chart + rancher-ali-operator charts).
func WaitForAlibabaProviderActivation(k *kubectl.Kubectl) error {
	if Provider != "alibaba" {
		return nil
	}
	// Step 1: Wait for UI extension pod (simulates Rancher UI refresh after repo add)
	By("Waiting for Alibaba UI extension pod to be ready (refresh Rancher)", func() {
		checkList := [][]string{{"cattle-ui-plugin-system", "app.kubernetes.io/instance=AlibabaCloud"}}
		Eventually(func() error { return testhelpersRancher.CheckPod(k, checkList) }, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil(), "Alibaba UI extension pod not ready")
	})
	// Step 2: Wait for operator pod
	By("Waiting for Alibaba operator pods to be ready", func() {
		checkList := [][]string{{"cattle-system", "app=rancher-ali-operator"}}
		Eventually(func() error { return testhelpersRancher.CheckPod(k, checkList) }, tools.SetTimeout(6*time.Minute), 30*time.Second).Should(BeNil(), "Alibaba operator pod not ready")
	})

	// Step 3: Wait for kontainerdriver AlibabaCloud to be Active in Rancher
	By("Waiting for AlibabaCloud kontainerdriver to be Active in Rancher", func() {
		Eventually(func() (string, error) {
			out, err := kubectl.RunWithoutErr(
				"get", "kontainerdrivers.management.cattle.io", "alibabacloud",
				"-o", "jsonpath={.status.conditions[?(@.type=='Active')].status}",
			)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(out), nil
		}, tools.SetTimeout(3*time.Minute), 15*time.Second).Should(Equal("True"), "AlibabaCloud kontainerdriver not Active")
	})

	GinkgoLogr.Info("Alibaba provider activation checks passed (pods ready, kontainerdriver Active)")
	return nil
}

// ActivateAlibabaProvider is a convenience wrapper to call WaitForAlibabaProviderActivation.
func ActivateAlibabaProvider(k *kubectl.Kubectl) error { return WaitForAlibabaProviderActivation(k) }

// VerifyAlibabaClusterRepo ensures ali-ui ClusterRepo exists and is ready.
func VerifyAlibabaClusterRepo(client *rancher.Client) error {
	err := AddClusterRepo(client, "ali-ui", "https://github.com/rancher/ali-ui", "gh-pages")
	if err != nil {
		return err
	}
	return nil
}

// VerifyUiPluginRepo ensures ali-ui ClusterRepo exists and is ready.
func VerifyUiPluginRepo(client *rancher.Client) error {
	err := AddClusterRepo(client, "ui-plugin-charts", "https://github.com/rancher/ui-plugin-charts", "main")
	if err != nil {
		return err
	}
	return nil
}

// LogAlibabaActivationSummary writes a simple activation summary.
func LogAlibabaActivationSummary() {
	GinkgoLogr.Info(fmt.Sprintf("Alibaba activation attempted; Provider=%s", Provider))
}
