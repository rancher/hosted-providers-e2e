package k8s_chart_support_upgrade_test

import (
	"fmt"
	"os"
	"testing"
	"time"

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
	testCaseID              int64
	k                       = kubectl.New()
)

func TestK8sChartSupportUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sChartSupportUpgrade Suite")
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

	By(fmt.Sprintf("Installing Rancher Manager %s", helpers.RancherFullVersion), func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(helpers.RancherFullVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "", "")
		helpers.CheckRancherDeployments(k)
	})

	By("Installing Alibaba operator charts", func() {
		helpers.InstallAlibabaOperatorCharts(k, os.Getenv("ALIBABA_OPERATOR_VERSION"), os.Getenv("ALIBABA_OPERATOR_REGISTRY"))
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
})

var _ = AfterEach(func() {
	// The test must restore the env to its original state, so we install rancher back to its original version and uninstall the operator charts
	// Restoring rancher back to its original state is necessary because in case DOWNSTREAM_CLUSTER_CLEANUP is set to false; in which case clusters will be retained for the next test.
	// Once the operator is uninstalled, it might be reinstalled since the cluster exists, and installing rancher back to its original state ensures that the version is not the one we want to test.
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
			Eventually(func() bool {
				isConnected, err := ctx.RancherAdminClient.IsConnected()
				if err != nil || !isConnected {
					return false
				}
				return true
			}, tools.SetTimeout(2*time.Minute), 10*time.Second).Should(BeTrue(), "Rancher client not connected after upgrade")
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
	By("upgrading the operator chart to match the new Rancher version", func() {
		// Alibaba operator is installed from OCI registry and not auto-upgraded by Rancher,
		// so we need to explicitly upgrade it after the Rancher upgrade.
		chartVersions := helpers.ListChartVersions("rancher-ali-operator")
		Expect(chartVersions).ToNot(BeEmpty(), "No chart versions found in the registry")
		latestChartVersion := chartVersions[0].DerivedVersion
		GinkgoLogr.Info(fmt.Sprintf("Upgrading operator chart from %s to %s", originalChartVersion, latestChartVersion))
		if helpers.VersionCompare(latestChartVersion, originalChartVersion) == 1 {
			helpers.UpdateOperatorChartsVersion(latestChartVersion)
		}
		upgradedChartVersion = helpers.GetCurrentOperatorChartVersion()
		Expect(upgradedChartVersion).ToNot(BeEmpty())
		Expect(helpers.VersionCompare(upgradedChartVersion, originalChartVersion)).To(BeNumerically(">=", 0))
		GinkgoLogr.Info("Upgraded chart version: " + upgradedChartVersion)
	})

	By("making sure the downstream cluster is ready", func() {
		var err error
		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

		// since no changes have been made to the cluster so far, we need reinstantiate AliConfig after fetching the cluster
		if helpers.IsImport {
			cluster.AliConfig = cluster.AliStatus.UpstreamSpec
		}
	})

	var latestVersion *string
	By(fmt.Sprintf("fetching a list of available k8s versions and ensure the v%s is present in the list and upgrading the cluster to it", k8sUpgradedVersion), func() {
		versions, err := helper.ListALIAllVersions(ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		Expect(versions).ToNot(BeEmpty())
		GinkgoLogr.Info(fmt.Sprintf("Available ACK versions: %v", versions))

		latestVersion = &versions[0]
		Expect(*latestVersion).To(ContainSubstring(k8sUpgradedVersion))
		Expect(helpers.VersionCompare(*latestVersion, cluster.Version.GitVersion)).To(BeNumerically("==", 1))

		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, *latestVersion, ctx.RancherAdminClient, true, true, true)
		Expect(err).To(BeNil())
	})

	var downgradeVersion string
	By("fetching a value to downgrade to", func() {
		downgradeVersion = helpers.GetDowngradeOperatorChartVersion(upgradedChartVersion)
		Expect(downgradeVersion).ToNot(BeEmpty(), "No downgrade version found for chart version %s", upgradedChartVersion)
		GinkgoLogr.Info("Downgrade version: " + downgradeVersion)
	})

	By("downgrading the chart version", func() {
		helpers.DowngradeProviderChart(downgradeVersion)
	})

	By("making a change to the cluster (scaling the node pool) to validate functionality after chart downgrade", func() {
		var err error
		configNodePools := cluster.AliConfig.NodePools
		initialNodeCount := *configNodePools[0].DesiredSize
		cluster, err = helper.ScaleNodePool(cluster, ctx.RancherAdminClient, initialNodeCount+1, true, true)
		Expect(err).To(BeNil())
	})

	By("uninstalling the operator chart", func() {
		helpers.UninstallOperatorCharts()
	})

	By("re-installing the operator chart and validating it is at the latest/upgraded version", func() {
		// Alibaba operator is not auto-managed by Rancher, so we explicitly re-install it.
		// UpdateOperatorChartsVersion won't work here because it iterates over ListOperatorChart()
		// which returns empty after uninstall.
		helpers.InstallAlibabaOperatorCharts(k, upgradedChartVersion, os.Getenv("ALIBABA_OPERATOR_REGISTRY"))

		By("ensuring that the chart is re-installed to the latest/upgraded version", func() {
			helpers.WaitUntilOperatorChartInstallation(upgradedChartVersion, "", 0)
		})
	})

	By("making a change(adding a nodepool) to the cluster to validate functionality after re-install", func() {
		currentNodePoolNumber := len(cluster.AliConfig.NodePools)
		var err error
		cluster, err = helper.AddNodePool(cluster, ctx.RancherAdminClient, 1, "", false, false)
		Expect(err).To(BeNil())

		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())

		// Check if the desired config has been applied in Rancher
		Eventually(func() int {
			cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.AliStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(20*time.Minute), 10*time.Second).Should(BeNumerically("==", currentNodePoolNumber+1))
	})
}
