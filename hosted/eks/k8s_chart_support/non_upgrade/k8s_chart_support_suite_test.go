package chart_support_test

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher/catalog"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx                     helpers.Context
	clusterName, k8sVersion string
	region                  = helpers.GetEKSRegion()
	testCaseID              int64
)

func TestK8sChartSupport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sChartSupport Suite")
}

var _ = BeforeSuite(func() {
	Expect(helpers.Kubeconfig).ToNot(BeEmpty())

	By("Adding the necessary chart repos", func() {
		err := kubectl.RunHelmBinaryWithCustomErr("repo", "add", catalog.RancherChartRepo, "https://charts.rancher.io")
		Expect(err).To(BeNil())
		err = kubectl.RunHelmBinaryWithCustomErr("repo", "add", fmt.Sprintf("rancher-%s", helpers.RancherChannel), fmt.Sprintf("https://releases.rancher.com/server-charts/%s", helpers.RancherChannel))
		Expect(err).To(BeNil())
	})

})

var _ = BeforeEach(func() {
	var err error
	ctx, err = helpers.CommonBeforeSuite(helpers.Provider)
	Expect(err).To(BeNil())
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)

	k8sVersion, err = helper.GetK8sVersion(ctx.RancherClient)
	Expect(err).To(BeNil())
	Expect(k8sVersion).ToNot(BeEmpty())

	GinkgoLogr.Info(fmt.Sprintf("Using EKS version %s", k8sVersion))

})

var _ = AfterEach(func() {

	By("Uninstalling the existing operator charts", func() {
		charts := helpers.ListOperatorChart()
		for _, chart := range charts {
			args := []string{"uninstall", chart.Name, "--namespace", helpers.CattleSystemNS}
			err := kubectl.RunHelmBinaryWithCustomErr(args...)
			Expect(err).To(BeNil())
		}
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

func commonchecks(ctx *helpers.Context, cluster *management.Cluster) {
	var originalChartVersion string

	By("checking the chart version", func() {
		oldCharts := helpers.ListOperatorChart()
		originalChartVersion = oldCharts[0].DerivedVersion
		Expect(originalChartVersion).ToNot(BeEmpty())
		GinkgoLogr.Info("Original chart version: " + originalChartVersion)
	})

	var downgradedVersion string
	By("obtaining a version to downgrade", func() {
		chart := helpers.ListOperatorChart()[0]
		chartVersions := helpers.ListChartVersions(chart.Name)
		for _, chartVersion := range chartVersions {
			if helpers.VersionCompare(chartVersion.DerivedVersion, originalChartVersion) == -1 {
				downgradedVersion = chartVersion.DerivedVersion
				break
			}
		}
		Expect(downgradedVersion).ToNot(BeEmpty())
		GinkgoLogr.Info("Downgrading to version: " + downgradedVersion)
	})

	By("downgrading the chart version", func() {
		newCharts := helpers.ListOperatorChart()
		for _, chart := range newCharts {
			err := kubectl.RunHelmBinaryWithCustomErr("upgrade", "--install", chart.Name, fmt.Sprintf("%s/%s", catalog.RancherChartRepo, chart.Name), "--namespace", helpers.CattleSystemNS, "--version", downgradedVersion, "--wait")
			Expect(err).To(BeNil())
		}
		// wait until the downgraded chart version is same as the old version
		Eventually(func() int {
			charts := helpers.ListOperatorChart()
			downgradedChartVersion := charts[0].DerivedVersion
			return helpers.VersionCompare(downgradedChartVersion, originalChartVersion)
		}, tools.SetTimeout(1*time.Minute), 5*time.Second).Should(BeNumerically("==", -1))

	})

	By("making a change to the cluster to validate functionality after chart downgrade", func() {
		initialNodeCount := *cluster.EKSConfig.NodeGroups[0].DesiredSize

		var err error
		cluster, err = helper.ScaleNodeGroup(cluster, ctx.RancherClient, initialNodeCount+1)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
		Expect(err).To(BeNil())
		for i := range cluster.EKSConfig.NodeGroups {
			Expect(*cluster.EKSConfig.NodeGroups[i].DesiredSize).To(BeNumerically("==", initialNodeCount+1))
		}
	})

	By("uninstalling the operator chart", func() {
		charts := helpers.ListOperatorChart()
		for _, chart := range charts {
			args := []string{"uninstall", chart.Name, "--namespace", helpers.CattleSystemNS}
			err := kubectl.RunHelmBinaryWithCustomErr(args...)
			Expect(err).To(BeNil())
		}
	})

	By("making a change(adding a nodepool) to the cluster to re-install the operator and validating it is re-installed to the latest version", func() {
		currentNodeGroupNumber := len(cluster.EKSConfig.NodeGroups)
		var err error
		cluster, err = helper.AddNodeGroup(cluster, 1, ctx.RancherClient)
		Expect(err).To(BeNil())

		By("ensuring that the chart is re-installed to the latest version", func() {
			Eventually(func() int {
				charts := helpers.ListOperatorChart()
				if len(charts) == 0 {
					return 10
				}
				reinstalledChartVersion := charts[0].DerivedVersion
				return helpers.VersionCompare(reinstalledChartVersion, originalChartVersion)
			}, tools.SetTimeout(1*time.Minute), 5*time.Second).Should(BeNumerically("==", 0))

		})

		err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
		Expect(err).To(BeNil())
		Expect(len(cluster.EKSConfig.NodeGroups)).To(BeNumerically("==", currentNodeGroupNumber+1))
	})

}
