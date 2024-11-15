/*
Copyright © 2022 - 2024 SUSE LLC

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

package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
)

var _ = Describe("Provision k3s cluster with Rancher", Label("install"), func() {
	// Create kubectl context
	// Default timeout is too small, so New() cannot be used
	k := &kubectl.Kubectl{
		Namespace:    "",
		PollTimeout:  tools.SetTimeout(300 * time.Second),
		PollInterval: 500 * time.Millisecond,
	}

	It("Install upstream k3s cluster", func() {
		if proxy != "" {
			By("Run local squid proxy in docker", func() {
				// Configure proxy before K3s installation if requested
				GinkgoLogr.Info("Starting squid proxy")

				cwd, _ := os.Getwd()
				GinkgoLogr.Info("Current working directory: " + cwd)

				out, err := exec.Command("docker", "run", "-d", "--rm", "--name", "squid_proxy",
					"--volume", cwd+"/.github/scripts/squid.conf:/etc/squid/squid.conf",
					"-p", "3128:3128", "ubuntu/squid").CombinedOutput()
				GinkgoWriter.Println(string(out))
				Expect(err).To(Not(HaveOccurred()))
			})

			By("Configure proxy in /etc/default/k3s", func() {
				k3sConfigPath := "/etc/default/k3s"
				k3sConfig := fmt.Sprintf(`HTTP_PROXY=http://%s
HTTPS_PROXY=http://%s
NO_PROXY=127.0.0.0/8,10.0.0.0/8,cattle-system.svc,172.16.0.0/12,192.168.0.0/16,.svc,.cluster.local`, proxyHost, proxyHost)
				// Write the proxy configuration to the file as root
				out, err := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | sudo tee %s", k3sConfig, k3sConfigPath)).CombinedOutput()
				GinkgoWriter.Println(string(out))
				Expect(err).To(Not(HaveOccurred()))
			})
		}

		By("Installing K3s", func() {
			// Get K3s installation script
			fileName := "k3s-install.sh"
			Eventually(func() error {
				return tools.GetFileFromURL("https://get.k3s.io", fileName, true)
			}, tools.SetTimeout(2*time.Minute), 10*time.Second).ShouldNot(HaveOccurred())

			// Set command and arguments
			installCmd := exec.Command("sh", fileName)
			// INSTALL_K3S_VERSION should be already set by the action or user's shell
			installCmd.Env = append(os.Environ(), "INSTALL_K3S_EXEC=--disable metrics-server --write-kubeconfig-mode 644")

			// Retry in case of (sporadic) failure...
			count := 1
			Eventually(func() error {
				// Execute K3s installation
				out, err := installCmd.CombinedOutput()
				GinkgoWriter.Printf("K3s installation loop %d:\n%s\n", count, out)
				count++
				return err
			}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(BeNil())
		})

		By("Starting K3s", func() {
			err := exec.Command("sudo", "systemctl", "start", "k3s").Run()
			Expect(err).To(Not(HaveOccurred()))

			// Delay few seconds before checking
			time.Sleep(tools.SetTimeout(5 * time.Second))
		})

		By("Waiting for K3s to be started", func() {
			// Wait for all pods to be started
			checkList := [][]string{
				{"kube-system", "app=local-path-provisioner"},
				{"kube-system", "k8s-app=kube-dns"},
				{"kube-system", "app.kubernetes.io/name=traefik"},
				{"kube-system", "svccontroller.k3s.cattle.io/svcname=traefik"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("Installing CertManager", func() {
			RunHelmCmdWithRetry("repo", "add", "jetstack", "https://charts.jetstack.io")
			RunHelmCmdWithRetry("repo", "update")

			// Set flags for cert-manager installation
			flags := []string{
				"upgrade", "--install", "cert-manager", "jetstack/cert-manager",
				"--namespace", "cert-manager",
				"--create-namespace",
				"--set", "crds.enabled=true",
				"--wait", "--wait-for-jobs",
			}

			if proxy != "" {
				flags = append(flags, "--set", "http_proxy=http://"+proxyHost,
					"--set", "https_proxy=http://"+proxyHost,
					"--set", "no_proxy=127.0.0.0/8\\,10.0.0.0/8\\,cattle-system.svc\\,172.16.0.0/12\\,192.168.0.0/16\\,.svc\\,.cluster.local")
			}
			GinkgoWriter.Printf("Helm flags: %v\n", flags)
			RunHelmCmdWithRetry(flags...)

			checkList := [][]string{
				{"cert-manager", "app.kubernetes.io/component=controller"},
				{"cert-manager", "app.kubernetes.io/component=webhook"},
				{"cert-manager", "app.kubernetes.io/component=cainjector"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("Installing Rancher Manager", func() {
			var proxyEnabled string
			if proxy != "" {
				proxyEnabled = "rancher"
			} else {
				proxyEnabled = "none"
			}

			var extraFlags []string
			if nightlyChart != "" {
				extraFlags = []string{
					"--set", "extraEnv[2].name=CATTLE_SKIP_HOSTED_CLUSTER_CHART_INSTALLATION",
					"--set-string", "extraEnv[2].value=true",
				}
			} else {
				extraFlags = nil
			}

			err := rancher.DeployRancherManager(rancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "none", proxyEnabled, extraFlags)
			Expect(err).To(Not(HaveOccurred()))

			// Wait for all pods to be started
			checkList := [][]string{
				{"cattle-system", "app=rancher"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("Waiting for fleet", func() {
			// Wait unit the kubectl command returns exit code 0
			count := 1
			Eventually(func() error {
				out, err := kubectl.Run("rollout", "status",
					"--namespace", "cattle-fleet-system",
					"deployment", "fleet-controller",
				)
				GinkgoWriter.Printf("Waiting for fleet-controller deployment, loop %d:\n%s\n", count, out)
				count++
				return err
			}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(Not(HaveOccurred()))

			checkList := [][]string{
				{"cattle-fleet-system", "app=fleet-controller"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("Waiting for rancher-webhook", func() {
			// Wait unit the kubectl command returns exit code 0
			count := 1
			Eventually(func() error {
				out, err := kubectl.Run("rollout", "status",
					"--namespace", "cattle-system",
					"deployment", "rancher-webhook",
				)
				GinkgoWriter.Printf("Waiting for rancher-webhook deployment, loop %d:\n%s\n", count, out)
				count++
				return err
			}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(Not(HaveOccurred()))

			checkList := [][]string{
				{"cattle-system", "app=rancher-webhook"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("Waiting for capi-controller-manager", func() {
			// Wait unit the kubectl command returns exit code 0
			count := 1
			Eventually(func() error {
				out, err := kubectl.Run("rollout", "status",
					"--namespace", "cattle-provisioning-capi-system",
					"deployment", "capi-controller-manager",
				)
				GinkgoWriter.Printf("Waiting for capi-controller-manager deployment, loop %d:\n%s\n", count, out)
				count++
				return err
			}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(Not(HaveOccurred()))

			checkList := [][]string{
				{"cattle-provisioning-capi-system", "cluster.x-k8s.io/provider=cluster-api"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		if nightlyChart != "" {
			By(fmt.Sprintf("Install nightly %s-operator via Helm", providerOperator), func() {
				buildDate := time.Now().Format("20060102")
				RunHelmCmdWithRetry("upgrade", "--install", providerOperator+"-operator-crds",
					"oci://ttl.sh/"+providerOperator+"-operator/rancher-"+providerOperator+"-operator-crd",
					"--version", buildDate)
				RunHelmCmdWithRetry("upgrade", "--install", providerOperator+"-operator",
					"oci://ttl.sh/"+providerOperator+"-operator/rancher-"+providerOperator+"-operator",
					"--version", buildDate, "--namespace", "cattle-system")
			})
		}
	})
})
