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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("Provision k3s cluster and Rancher", Label("install"), func() {
	k := kubectl.New()

	It("Install upstream k3s cluster", func() {
		By("Installing K3S", func() {
			helpers.InstallK3S(k, k3sVersion, proxy, proxyHost)
		})

		By("Installing CertManager", func() {
			helpers.InstallCertManager(k, proxy, proxyHost)
		})

		usesStagedOperatorCharts := providerOperator == "aks" || providerOperator == "eks" || providerOperator == "gke"
		if skipInstallRancher != "true" {
			By("Installing Rancher Manager", func() {
				skipHostedChartInstall := nightlyChart == "enabled" || usesStagedOperatorCharts
				chartInstallMode := ""
				if skipHostedChartInstall {
					chartInstallMode = "enabled"
				}
				helpers.InstallRancherManager(k, rancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, proxy, chartInstallMode)
			})

			By("Checking Rancher Deployments", func() {
				helpers.CheckRancherDeployments(k)
			})
		} else {
			GinkgoLogr.Info("Skipping Rancher Manager installation; SKIP_RANCHER_INSTALL=\"true\"")
		}

		if nightlyChart == "enabled" {
			By(fmt.Sprintf("Install nightly rancher-%s-operator via Helm", providerOperator), func() {
				if providerOperator != "alibaba" {
					// Get the current date to use as the build date
					buildDate := time.Now().Format("20060102")

					RunHelmCmdWithRetry("upgrade", "--install", "rancher-"+providerOperator+"-operator-crds",
						"oci://ghcr.io/rancher/rancher-"+providerOperator+"-operator-crd-chart/rancher-"+providerOperator+"-operator-crd",
						"--version", buildDate)
					RunHelmCmdWithRetry("upgrade", "--install", "rancher-"+providerOperator+"-operator",
						"oci://ghcr.io/rancher/rancher-"+providerOperator+"-operator-chart/rancher-"+providerOperator+"-operator",
						"--version", buildDate, "--namespace", "cattle-system")
				}
			})
		} else if usesStagedOperatorCharts {
			By(fmt.Sprintf("Install staged rancher-%s-operator charts", providerOperator), func() {
				chartVersion, err := helpers.GetStagedOperatorChartVersion(providerOperator, helpers.RancherFullVersion)
				Expect(err).NotTo(HaveOccurred())
				GinkgoLogr.Info(fmt.Sprintf("Installing staged provider charts at version %s", chartVersion))

				operatorChartName := "rancher-" + providerOperator + "-operator"
				chartRegistry := helpers.StagingOperatorChartRegistry
				RunHelmCmdWithRetry("upgrade", "--install", operatorChartName+"-crd",
					chartRegistry+"/"+operatorChartName+"-crd",
					"--namespace", "cattle-system", "--version", chartVersion, "--wait")
				RunHelmCmdWithRetry("upgrade", "--install", operatorChartName,
					chartRegistry+"/"+operatorChartName,
					"--namespace", "cattle-system", "--version", chartVersion, "--wait")
			})
		}

		// Install Alibaba components if running Alibaba tests
		if providerOperator == "alibaba" && skipInstallRancher != "true" {
			By("Installing Alibaba operator charts (CRD and operator)", func() {
				helpers.InstallAlibabaOperatorCharts(k, os.Getenv("ALIBABA_OPERATOR_VERSION"), os.Getenv("ALIBABA_OPERATOR_REGISTRY"))
			})
		}
	})
})
