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
	"github.com/rancher/shepherd/clients/rancher"
	"github.com/rancher/shepherd/pkg/config"
	"github.com/rancher/shepherd/pkg/session"
	"github.com/rancher/tests/actions/pipeline"

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

		if skipInstallRancher != "true" {
			By("Installing Rancher Manager", func() {
				helpers.InstallRancherManager(k, rancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, proxy, nightlyChart)
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
		}

		// Setup Alibaba provider if running Alibaba tests
		if providerOperator == "alibaba" && skipInstallRancher != "true" {
			By("Making Rancher ready for Alibaba cluster creation", func() {
				// Initialize Rancher config and create admin token
				rancherConfig := new(rancher.Config)
				config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)
				rancherConfig.Host = rancherHostname

				// Create admin token
				time.Sleep(2 * time.Second)
				var err error
				rancherConfig.AdminToken, err = pipeline.CreateAdminToken(helpers.RancherPassword, rancherConfig)
				Expect(err).To(BeNil(), "Failed to create admin token")
				testSession := session.NewSession()
				rancherAdminClient, err := rancher.NewClient(rancherConfig.AdminToken, testSession)
				Expect(err).To(BeNil(), "Failed to create Rancher client for Alibaba setup")

				// Get chart version and registry from environment or use defaults
				chartVersion := os.Getenv("ALIBABA_OPERATOR_VERSION")
				chartRegistry := os.Getenv("ALIBABA_OPERATOR_REGISTRY")

				// Run the complete Alibaba setup including credential creation
				cloudCredID, err := helpers.MakeRancherReadyForAlibabaClusterCreation(rancherAdminClient, k, chartVersion, chartRegistry)
				Expect(err).To(BeNil(), "Failed to make Rancher ready for Alibaba cluster creation")

				GinkgoLogr.Info(fmt.Sprintf("✓ Rancher is ready for Alibaba cluster creation. Cloud Credential ID: %s", cloudCredID))
			})
		}
	})
})
