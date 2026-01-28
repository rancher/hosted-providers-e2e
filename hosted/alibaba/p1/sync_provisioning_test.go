package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncProvisioning", func() {
	var k8sVersion string
	var upgradeToVersion string

	BeforeEach(func() {
		var err error
		// Get the lower version for initial cluster creation (forUpgrade=true)
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
		Expect(err).NotTo(HaveOccurred())
		// Get the highest version for upgrade target (forUpgrade=false)
		upgradeToVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s, will upgrade to %s", k8sVersion, clusterName, upgradeToVersion))

		GinkgoLogr.Info(fmt.Sprintf("Creating cluster %s with kubernetes version %s", clusterName, k8sVersion))
		cluster, err = helper.CreateAlibabaHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		csClient, err = helper.CreateAliClient(region)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if ctx.ClusterCleanup && (cluster != nil && cluster.ID != "") {
			GinkgoLogr.Info(fmt.Sprintf("Deleting cluster %s", clusterName))
			err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			GinkgoLogr.Info(fmt.Sprintf("Skipping downstream cluster deletion: %s", clusterName))
		}
	})

	It("Should successfully sync k8s version upgrade of cluster from Alibaba to Rancher", func() {
		testCaseID = -1 // Qase TestCase ID
		syncK8sVersionUpgradeCheck(cluster, csClient, ctx.RancherAdminClient, upgradeToVersion)
	})

	It("Should successfully sync k8s version upgrade of cluster from Rancher to Alibaba", func() {
		testCaseID = -1 // Qase TestCase ID
		syncK8sVersionUpgradeFromRancher(cluster, csClient, ctx.RancherAdminClient, upgradeToVersion)
	})

	It("Should successfully sync nodepool changes from Alibaba to Rancher", func() {
		testCaseID = -1 // Qase TestCase ID
		aliNodePoolSyncCheck(cluster, csClient, ctx.RancherAdminClient, k8sVersion)
	})
})
