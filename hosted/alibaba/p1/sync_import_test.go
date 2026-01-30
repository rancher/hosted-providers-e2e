package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncImport", func() {
	var k8sVersion string
	var upgradeToVersion string
	var clusterId string

	BeforeEach(func() {
		var err error
		csClient, err = helper.CreateAliClient(region)
		Expect(err).To(BeNil())

		// Get the lower version for initial cluster creation (forUpgrade=true)
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
		Expect(err).To(BeNil())
		// Get the highest version for upgrade target (forUpgrade=false)
		upgradeToVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).To(BeNil())

		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s, will upgrade to %s", k8sVersion, clusterName, upgradeToVersion))

		GinkgoLogr.Info(fmt.Sprintf("Creating cluster %s with kubernetes version %s", clusterName, k8sVersion))
		resourceGroupId := helpers.GetACKResourceGroupID()
		clusterId, err = helper.CreateACKClusterOnAlibaba(csClient, region, clusterName, k8sVersion, "1", resourceGroupId, helpers.GetCommonMetadataLabels())
		Expect(err).To(BeNil())

		cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			var err error
			if cluster != nil && cluster.ID != "" {
				err = helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
			err = helper.DeleteACKClusterOnAlibaba(csClient, clusterId)
			Expect(err).To(BeNil())

		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	It("should successfully upgrade k8s version from Alibaba and sync back to rancher", func() {
		testCaseID = -1
		syncK8sVersionUpgradeCheck(cluster, csClient, ctx.RancherAdminClient, upgradeToVersion)
	})

	It("should successfully upgrade k8s version from rancher and sync back to alibaba", func() {
		testCaseID = -1
		syncK8sVersionUpgradeFromRancher(cluster, csClient, ctx.RancherAdminClient, upgradeToVersion)
	})

	It("should successfully sync nodepool changes from Alibaba to Rancher", func() {
		testCaseID = -1
		aliNodePoolSyncCheck(cluster, csClient, ctx.RancherAdminClient, "")
	})
})
