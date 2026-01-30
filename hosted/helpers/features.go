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
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

// enableProviderViaFeatureFlag is the fallback mechanism to enable a provider using feature flags.
func enableProviderViaFeatureFlag(client *rancher.Client, providerName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	feature, err := client.Management.Feature.ByID(providerName)
	if err != nil {
		// Rancher may not expose a feature for this provider. Treat as non-fatal.
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

// EnableProvider ensures a hosted provider is enabled, trying the `kev2-operators` setting first and falling back to feature flags.
func EnableProvider(client *rancher.Client, providerName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	GinkgoLogr.Info(fmt.Sprintf("Ensuring provider %s is enabled", providerName))

	// Try to enable via kev2-operators setting first.
	setting, err := client.Management.Setting.ByID("kev2-operators")
	if err == nil {
		GinkgoLogr.Info("Found kev2-operators setting, attempting to enable provider.")

		type KEv2Operator struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
		}

		var operators []KEv2Operator
		if setting.Value != "" {
			if err := json.Unmarshal([]byte(setting.Value), &operators); err != nil {
				GinkgoLogr.Info(fmt.Sprintf("[WARN] failed to unmarshal kev2-operators setting: %v; falling back to feature flag", err))
				return enableProviderViaFeatureFlag(client, providerName)
			}
		}

		found, changed := false, false
		for i, op := range operators {
			if op.Name == providerName {
				found = true
				if !op.Active {
					operators[i].Active, changed = true, true
				}
				break
			}
		}
		if !found {
			operators, changed = append(operators, KEv2Operator{Name: providerName, Active: true}), true
		}
		if !changed {
			GinkgoLogr.Info("Provider already active in kev2-operators setting.")
			return nil
		}

		newValue, err := json.Marshal(operators)
		if err != nil {
			return fmt.Errorf("marshal kev2-operators: %w", err)
		}
		setting.Value = string(newValue)

		if _, err := client.Management.Setting.Update(setting, setting); err != nil {
			return fmt.Errorf("update kev2-operators setting: %w", err)
		}

		// Verification retry loop
		err = retry(ctx, 10, 3*time.Second, func() (bool, error) {
			latest, e := client.Management.Setting.ByID("kev2-operators")
			if e != nil {
				return false, e
			}
			var latestOps []KEv2Operator
			if latest.Value == "" {
				return false, nil
			}
			if e := json.Unmarshal([]byte(latest.Value), &latestOps); e != nil {
				return false, e
			}
			for _, op := range latestOps {
				if op.Name == providerName && op.Active {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("provider %s not enabled in kev2-operators: %w", providerName, err)
		}

		GinkgoLogr.Info(fmt.Sprintf("Provider %s enabled via kev2-operators", providerName))
		return nil
	}

	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return fmt.Errorf("get kev2-operators setting: %w", err)
	}

	// Fallback to feature flag if kev2-operators setting is not found.
	GinkgoLogr.Info("kev2-operators setting not found, falling back to feature flag.")
	return enableProviderViaFeatureFlag(client, providerName)
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

// InstallAlibabaUIExtension installs the AlibabaCloud (ACK Provisioning) UI extension chart.
// This function adds the necessary repos and installs the AlibabaCloud chart.
func InstallAlibabaUIExtension(client *rancher.Client) error {
	if Provider != "alibaba" {
		return nil
	}

	GinkgoLogr.Info("Installing Alibaba UI extension")

	// Step 1: Add ui-plugin-charts ClusterRepo
	By("Adding ui-plugin-charts ClusterRepo", func() {
		err := VerifyUiPluginRepo(client)
		Expect(err).To(BeNil(), "Failed to add ui-plugin-charts ClusterRepo")
	})

	// Step 2: Add ali-ui ClusterRepo
	By("Adding ali-ui ClusterRepo", func() {
		err := VerifyAlibabaClusterRepo(client)
		Expect(err).To(BeNil(), "Failed to add ali-ui ClusterRepo")
	})

	// Step 3: Install AlibabaCloud chart from ali-ui repo
	// This simulates installing the extension from Extensions page
	By("Installing AlibabaCloud UI extension chart", func() {
		// Use kubectl to install the chart via Helm
		err := kubectl.RunHelmBinaryWithCustomErr(
			"install", "alibabacloud-ack",
			"ali-ui/AlibabaCloud",
			"--namespace", "cattle-ui-plugin-system",
			"--create-namespace",
			"--wait",
		)
		if err != nil {
			// Chart might already be installed, check if it exists
			GinkgoLogr.Info(fmt.Sprintf("Chart installation returned error (might already exist): %v", err))
		}
	})

	GinkgoLogr.Info("Alibaba UI extension installation initiated")
	return nil
}

// SetupAlibabaProvider is the main function that orchestrates all steps to enable Alibaba provider.
// This automates:
// 1. Installing ali-operator-crd and ali-operator charts
// 2. Adding ClusterRepos (ui-plugin-charts and ali-ui)
// 3. Installing AlibabaCloud UI extension
// 4. Waiting for all pods to be ready
// 5. Enabling the provider in Rancher
func SetupAlibabaProvider(client *rancher.Client, k *kubectl.Kubectl, chartVersion, chartRegistry string) error {
	if Provider != "alibaba" {
		GinkgoLogr.Info("Skipping Alibaba provider setup; not running Alibaba tests")
		return nil
	}

	GinkgoLogr.Info("=== Starting Alibaba Provider Setup ===")

	// Step 1: Install operator charts via Helm
	By("Step 1: Installing Alibaba operator charts", func() {
		InstallAlibabaOperatorCharts(k, chartVersion, chartRegistry)
	})

	// Step 2: Install UI extension
	By("Step 2: Installing Alibaba UI extension", func() {
		err := InstallAlibabaUIExtension(client)
		Expect(err).To(BeNil(), "Failed to install Alibaba UI extension")
	})

	// Step 3: Wait for all components to be ready (simulates Rancher UI reload)
	By("Step 3: Waiting for Alibaba provider activation", func() {
		err := WaitForAlibabaProviderActivation(k)
		Expect(err).To(BeNil(), "Alibaba provider activation failed")
	})

	// Step 4: Enable provider via kev2-operators setting or feature flag
	By("Step 4: Enabling Alibaba Cloud provider in Rancher", func() {
		err := EnableProvider(client, "alibabacloud")
		Expect(err).To(BeNil(), "Failed to enable Alibaba provider")
	})

	GinkgoLogr.Info("=== Alibaba Provider Setup Complete ===")
	GinkgoLogr.Info("Alibaba Cloud clusters can now be created and imported")

	return nil
}

// MakeRancherReadyForAlibabaClusterCreation prepares Rancher completely for creating Alibaba clusters.
// This is the complete end-to-end setup that includes:
// 1. Setting up the Alibaba provider (operator, UI extension, activation)
// 2. Creating cloud credentials in Rancher
// 3. Verifying all prerequisites are met
// Returns the cloud credential ID that can be used for cluster creation.
func MakeRancherReadyForAlibabaClusterCreation(client *rancher.Client, k *kubectl.Kubectl, chartVersion, chartRegistry string) (cloudCredID string, err error) {
	if Provider != "alibaba" {
		GinkgoLogr.Info("Skipping Alibaba cluster creation setup; not running Alibaba tests")
		return "", nil
	}

	GinkgoLogr.Info("=== Making Rancher Ready for Alibaba Cluster Creation ===")

	// Step 1: Setup Alibaba provider (charts, UI, activation)
	By("Step 1: Setting up Alibaba provider infrastructure", func() {
		err := SetupAlibabaProvider(client, k, chartVersion, chartRegistry)
		Expect(err).To(BeNil(), "Failed to setup Alibaba provider")
	})

	// Step 2: Create Alibaba cloud credentials
	var credID string
	By("Step 2: Creating Alibaba Cloud credentials in Rancher", func() {
		GinkgoLogr.Info("Creating Alibaba cloud credentials from environment variables")

		// Verify environment variables are set
		accessKeyID := os.Getenv("ALIBABA_ACCESS_KEY_ID")
		accessKeySecret := os.Getenv("ALIBABA_ACCESS_KEY_SECRET")

		if accessKeyID == "" || accessKeySecret == "" {
			Expect(fmt.Errorf("ALIBABA_ACCESS_KEY_ID and ALIBABA_ACCESS_KEY_SECRET must be set")).To(BeNil())
		}

		GinkgoLogr.Info(fmt.Sprintf("Using Alibaba Access Key ID: %s (first 10 chars)", accessKeyID[:min(10, len(accessKeyID))]))

		// Create cloud credentials using the existing helper
		var createErr error
		credID, createErr = CreateCloudCredentials(client)
		Expect(createErr).To(BeNil(), "Failed to create Alibaba cloud credentials")

		GinkgoLogr.Info(fmt.Sprintf("Cloud credentials created successfully: %s", credID))
	})

	// Step 3: Verify credentials are accessible
	By("Step 3: Verifying cloud credentials are accessible", func() {
		// Verify the credential exists in Rancher
		Eventually(func() error {
			_, err := client.Steve.SteveType("provisioning.cattle.io.cloudcredential").ByID(credID)
			return err
		}, tools.SetTimeout(30*time.Second), 5*time.Second).Should(BeNil(), "Cloud credentials not found")

		GinkgoLogr.Info("Cloud credentials verified successfully")
	})

	// Step 4: Verify all prerequisites for cluster creation
	By("Step 4: Verifying all prerequisites for cluster creation", func() {
		// Check operator is running
		checkList := [][]string{{"cattle-system", "app=rancher-ali-operator"}}
		err := testhelpersRancher.CheckPod(k, checkList)
		Expect(err).To(BeNil(), "Alibaba operator pods not running")

		// Check UI extension is running
		checkList = [][]string{{"cattle-ui-plugin-system", "app.kubernetes.io/instance=AlibabaCloud"}}
		err = testhelpersRancher.CheckPod(k, checkList)
		Expect(err).To(BeNil(), "Alibaba UI extension pod not running")

		// Check kontainerdriver is Active
		out, err := kubectl.RunWithoutErr(
			"get", "kontainerdrivers.management.cattle.io", "alibabacloud",
			"-o", "jsonpath={.status.conditions[?(@.type=='Active')].status}",
		)
		Expect(err).To(BeNil(), "Failed to check kontainerdriver status")
		Expect(strings.TrimSpace(out)).To(Equal("True"), "AlibabaCloud kontainerdriver not Active")

		GinkgoLogr.Info("All prerequisites verified successfully")
	})

	GinkgoLogr.Info("=== Rancher is Ready for Alibaba Cluster Creation ===")
	GinkgoLogr.Info(fmt.Sprintf("Cloud Credential ID: %s", credID))
	GinkgoLogr.Info("You can now create or import Alibaba ACK clusters")

	return credID, nil
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LogAlibabaActivationSummary writes a simple activation summary.
func LogAlibabaActivationSummary() {
	GinkgoLogr.Info(fmt.Sprintf("Alibaba activation attempted; Provider=%s", Provider))
}
