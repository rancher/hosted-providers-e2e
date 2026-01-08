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
	})

	AfterEach(func() {
		if ctx.ClusterCleanup && (cluster != nil && cluster.ID != "") {
			err := helper.DeleteALIHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			GinkgoLogr.Info(fmt.Sprintf("Skipping downstream cluster deletion: %s", clusterName))
		}
	})

	It("Should successfully upgrade k8s version of cluster from Alibaba", func() {
		testCaseID = -1 // Qase TestCase ID
		syncK8sVersionUpgradeCheck(cluster, ctx.RancherAdminClient, true, k8sVersion, upgradeToVersion)
	})

	It("Should successfully sync nodepool changes from Alibaba to Rancher", func() {
		testCaseID = -1 // Qase TestCase ID
		aliSyncCheck(cluster, ctx.RancherAdminClient, upgradeToVersion)
	})
})
