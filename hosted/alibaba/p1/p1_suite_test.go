package p1_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	cs "github.com/alibabacloud-go/cs-20151215/v5/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	"github.com/rancher/shepherd/clients/rancher"
)

var (
	ctx             helpers.RancherContext
	cluster         *management.Cluster
	clusterName     string
	testCaseID      int64
	csClient        *cs.Client
	region          = helpers.GetACKRegion()
	resourceGroupId = helpers.GetACKResourceGroupID()
)

func TestP1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P1 Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	helpers.CommonSynchronizedBeforeSuite()
	return nil
}, func() {
	ctx = helpers.CommonBeforeSuite()
	csClient = helpers.GetCSClient()
})

var _ = BeforeEach(func() {
	// Setting this to nil ensures we do not use the `cluster` variable value from another test running in parallel with this one.
	cluster = nil
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func upgradeK8sVersionFromAlibabaCheck(csClient *cs.Client, clusterId string, client *rancher.Client, upgradeToVersion string) {
	By("upgrading cluster k8s version from Alibaba", func() {
		GinkgoLogr.Info(fmt.Sprintf("Upgrading cluster on alibaba console to K8s version %s", upgradeToVersion))
		err := helper.UpgradeACKOnAlibaba(csClient, clusterId, upgradeToVersion)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Waiting for upgraded k8s version to sync on rancher..."))
			return cluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
		}, "20m", "5s").Should(BeTrue(), "Timed out while waiting for upgrade to appear in UpstreamSpec")
	})
}

func upgradeK8sVersionFromRancherCheck(cluster *management.Cluster, client *rancher.Client, upgradeToVersion string, csClient *cs.Client, clusterId string) {
	By("upgrading cluster k8s version from Rancher", func() {
		if helpers.IsImport {
			// if the cluster is imported, update the AliConfig value to match UpstreamSpec so that it can perform upcoming checks correctly
			cluster.AliConfig = cluster.AliStatus.UpstreamSpec
		}
		var err error
		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, client, true, true, true)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			clusterResp, err := helper.CheckClusterK8sVersionOnAlibaba(csClient, clusterId)
			Expect(err).NotTo(HaveOccurred())
			current := tea.StringValue(clusterResp.Body.CurrentVersion)
			GinkgoLogr.Info(fmt.Sprintf("Waiting for upgraded k8s version to sync on alibaba..."))
			return current == upgradeToVersion
		}, "20m", "5s").Should(BeTrue(), "Timed out while waiting for upgrade to appear on alibaba")
	})
}

func alibabaSyncCheck(cluster *management.Cluster, client *rancher.Client, csClient *cs.Client, clusterId string, resourceGroupId string) {
	const (
		npName          = "syncnodepool"
		nodeCount int64 = 1
	)
	var nodePoolResp *cs.CreateClusterNodePoolResponse

	// Using upstreamSpec so that it also works with import tests
	currentNPCount := len(cluster.AliStatus.UpstreamSpec.NodePools)
	By("Adding a nodepool", func() {
		var err error
		err, nodePoolResp = helper.AddNodePoolOnAlibaba(csClient, npName, clusterId, nodeCount, resourceGroupId)
		Expect(err).To(BeNil())
		fmt.Println("Waiting for new nodepool to sync on rancher...")
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			if len(cluster.AliStatus.UpstreamSpec.NodePools) == currentNPCount {
				// Return early if the nodepool count hasn't changed.
				return false
			}
			for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
				if nodepool.Name == npName && len(cluster.AliStatus.UpstreamSpec.NodePools) == currentNPCount+1 {
					return true
				}
			}
			return false
		}, "15m", "10s").Should(BeTrue(), "Timed out while waiting for new nodepool to appear in UpstreamSpec...")
	})

	By("Scaling the nodepool", func() {
		const scaleCount = 2
		err := helper.ScaleNodePoolOnAlibaba(csClient, clusterId, scaleCount, nodePoolResp)
		Expect(err).To(BeNil())
		fmt.Println("Waiting for scaled nodes to sync on rancher...")
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
				if nodepool.Name == npName {
					return tea.Int64Value(nodepool.DesiredSize) == int64(scaleCount+1)
				}
			}
			return false
		}, "15m", "10s").Should(BeTrue(), "Timed out while waiting for Scale up to appear in UpstreamSpec...")
	})

	By("Deleting a nodepool", func() {
		err := helper.DeleteNodePoolOnAlibaba(csClient, npName, clusterId, nodePoolResp)
		Expect(err).To(BeNil())
		fmt.Println("Waiting for nodepool deletion to sync on rancher...")
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			if len(cluster.AliStatus.UpstreamSpec.NodePools) != currentNPCount {
				//Return early if the nodepool count is not back to its original state
				return false
			}
			for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
				if nodepool.Name == npName {
					return false
				}
			}
			return true
		}, "15m", "10s").Should(BeTrue(), "Timed out while waiting for nodepool deletion to appear in UpstreamSpec...")

		//Check AliConfig if the cluster is Rancher-provisioned
		if !helpers.IsImport {
			Expect(cluster.AliConfig.NodePools).To(HaveLen(currentNPCount))
		}
	})
}
