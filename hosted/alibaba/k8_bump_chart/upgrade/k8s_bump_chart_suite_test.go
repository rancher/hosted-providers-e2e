/*
Copyright © 2025 - 2026 SUSE LLC

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

package k8s_bump_chart_test

import (
	"fmt"
	"testing"
	"time"

	cs "github.com/alibabacloud-go/cs-20151215/v5/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	nodestat "github.com/rancher/shepherd/extensions/nodes"
	"github.com/rancher/shepherd/extensions/workloads/pods"
	"github.com/rancher/shepherd/pkg/config"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx                     helpers.RancherContext
	clusterName, k8sVersion string
	region                  = helpers.GetACKRegion()
	resourceGroupId         = helpers.GetACKResourceGroupID()
	csClient                *cs.Client
	testCaseID              int64
	k                       = kubectl.New()
)

func TestK8sBumpChart(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sBumpChart Suite")
}

var _ = BeforeEach(func() {
	// For upgrade tests, the rancher version should not be an unreleased version (for e.g. 2.9-head)
	Expect(helpers.RancherFullVersion).To(SatisfyAll(Not(BeEmpty()), Not(ContainSubstring("devel"))))
	Expect(helpers.RancherUpgradeFullVersion).ToNot(BeEmpty())
	Expect(helpers.K8sUpgradedMinorVersion).ToNot(BeEmpty())
	Expect(helpers.Kubeconfig).ToNot(BeEmpty())

	By("Adding the necessary chart repos", func() {
		helpers.AddRancherCharts()
	})

	By("Installing CertManager", func() {
		helpers.InstallCertManager(k, "none", "")
	})

	By(fmt.Sprintf("Installing Rancher Manager %s", helpers.RancherFullVersion), func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(helpers.RancherFullVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "", "")
		helpers.CheckRancherDeployments(k)
	})

	helpers.CommonSynchronizedBeforeSuite()
	ctx = helpers.CommonBeforeSuite()

	By("creating and using a more permanent token", func() {
		token, err := ctx.RancherAdminClient.Management.Token.Create(&management.Token{})
		Expect(err).NotTo(HaveOccurred())
		rancherConfig := new(rancher.Config)
		config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)
		rancherConfig.AdminToken = token.Token
		config.UpdateConfig(rancher.ConfigurationFileKey, rancherConfig)

		rancherAdminClient, err := rancher.NewClient(rancherConfig.AdminToken, ctx.Session)
		Expect(err).To(BeNil())
		ctx.RancherAdminClient = rancherAdminClient
	})

	var err error
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
	Expect(err).To(BeNil())
	Expect(k8sVersion).ToNot(BeEmpty())
	GinkgoLogr.Info(fmt.Sprintf("Using ACK version %s for cluster %s", k8sVersion, clusterName))

	csClient = helpers.GetCSClient()
})

var _ = AfterEach(func() {
	By(fmt.Sprintf("Installing Rancher back to its original version %s", helpers.RancherFullVersion), func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(helpers.RancherFullVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "", "")
		helpers.CheckRancherDeployments(k)
	})

	By("Uninstalling the existing operator charts", func() {
		helpers.UninstallOperatorCharts()
	})
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func commonchecks(ctx *helpers.RancherContext, cluster *management.Cluster, clusterName, rancherUpgradedVersion, k8sUpgradedVersion string) {
	helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

	var originalChartVersion string
	By("checking the chart version", func() {
		originalChartVersion = helpers.GetCurrentOperatorChartVersion()
		Expect(originalChartVersion).ToNot(BeEmpty())
		GinkgoLogr.Info("Original chart version: " + originalChartVersion)
	})

	By(fmt.Sprintf("upgrading rancher to %v", rancherUpgradedVersion), func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(rancherUpgradedVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "none", "none")
		helpers.CheckRancherDeployments(k)

		By("ensuring operator pods are also up", func() {
			Eventually(func() error {
				return k.WaitForNamespaceWithPod(helpers.CattleSystemNS, fmt.Sprintf("ke.cattle.io/operator=%s", helpers.Provider))
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("ensuring the rancher client is connected", func() {
			isConnected, err := ctx.RancherAdminClient.IsConnected()
			Expect(err).To(BeNil())
			Expect(isConnected).To(BeTrue())
		})
	})

	By("making sure the local cluster is ready", func() {
		const localClusterID = "local"
		By("checking all management nodes are ready", func() {
			err := nodestat.AllManagementNodeReady(ctx.RancherAdminClient, localClusterID, helpers.Timeout)
			Expect(err).To(BeNil())
		})

		By("checking all pods are ready", func() {
			podErrors := pods.StatusPods(ctx.RancherAdminClient, localClusterID)
			Expect(podErrors).To(BeEmpty())
		})
	})

	var upgradedChartVersion string
	By("checking the chart version and validating it is > the old version", func() {
		helpers.WaitUntilOperatorChartInstallation(originalChartVersion, "==", 1)
		upgradedChartVersion = helpers.GetCurrentOperatorChartVersion()
		GinkgoLogr.Info("Upgraded chart version: " + upgradedChartVersion)
	})

	By("making sure the downstream cluster is ready", func() {
		var err error
		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

		if helpers.IsImport {
			cluster.AliConfig = cluster.AliStatus.UpstreamSpec
		}
	})

	var latestVersion *string
	By(fmt.Sprintf("fetching a list of available k8s versions and ensuring v%s is present and upgrading the cluster to it", k8sUpgradedVersion), func() {
		allVersions, err := helper.ListALIAllVersions(ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		Expect(allVersions).ToNot(BeEmpty())
		GinkgoLogr.Info(fmt.Sprintf("Available ACK versions: %v", allVersions))

		var upgradeVersions []string
		for _, v := range allVersions {
			if helpers.VersionCompare(v, cluster.Version.GitVersion) > 0 {
				upgradeVersions = append(upgradeVersions, v)
			}
		}
		Expect(upgradeVersions).ToNot(BeEmpty())

		latestVersion = &upgradeVersions[0]
		Expect(*latestVersion).To(ContainSubstring(k8sUpgradedVersion))

		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, *latestVersion, ctx.RancherAdminClient, true, true, true)
		Expect(err).To(BeNil())
	})

	var downgradeVersion string
	By("fetching a value to downgrade to", func() {
		downgradeVersion = helpers.GetDowngradeOperatorChartVersion(upgradedChartVersion)
	})

	By("downgrading the chart version", func() {
		helpers.DowngradeProviderChart(downgradeVersion)
	})

	By("making a change to the cluster (scaling the node up) to validate functionality after chart downgrade", func() {
		var err error
		initialNodeCount := *cluster.AliConfig.NodePools[0].DesiredSize
		cluster, err = helper.ScaleNodePool(cluster, ctx.RancherAdminClient, initialNodeCount+1, true, true)
		Expect(err).To(BeNil())
	})

	By("uninstalling the operator chart", func() {
		helpers.UninstallOperatorCharts()
	})

	By("making a change (adding a nodepool) to the cluster to re-install the operator and validating it is re-installed to the latest/upgraded version", func() {
		currentNodePoolNumber := len(cluster.AliConfig.NodePools)
		var err error
		cluster, err = helper.AddNodePool(cluster, ctx.RancherAdminClient, 1, "", false, false)
		Expect(err).To(BeNil())

		By("ensuring that the chart is re-installed to the latest/upgraded version", func() {
			helpers.WaitUntilOperatorChartInstallation(upgradedChartVersion, "", 0)
		})

		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())

		Eventually(func() int {
			cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.AliStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(20*time.Minute), 10*time.Second).Should(BeNumerically("==", currentNodePoolNumber+1))
	})
}
