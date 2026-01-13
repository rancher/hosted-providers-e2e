package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncImport", func() {
	var clusterId string
	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
			err := helper.DeleteACKClusteronAlibaba(csClient, clusterId)
			Expect(err).To(BeNil())

		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	When("a cluster is created and imported for upgrade", func() {
		var availableUpgradeVersions []string
		BeforeEach(func() {
			k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, true)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			clusterId, err = helper.CreateACKClusterOnAlibaba(csClient, region, clusterName, k8sVersion, "1", resourceGroupId, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			availableUpgradeVersions, err = helper.ListALIAllVersions(ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully upgrade k8s version from Alibaba and sync back to rancher", func() {
			testCaseID = 294
			upgradeK8sVersionFromAlibabaCheck(csClient, clusterId, ctx.RancherAdminClient, availableUpgradeVersions[0])
		})

		It("should successfully upgrade k8s version from rancher and sync back to alibaba", func() {
			testCaseID = 294
			upgradeK8sVersionFromRancherCheck(cluster, ctx.RancherAdminClient, availableUpgradeVersions[0], csClient, clusterId)
		})
	})

	When("a cluster is created and imported", func() {
		BeforeEach(func() {
			k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, false)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			clusterId, err = helper.CreateACKClusterOnAlibaba(csClient, region, clusterName, k8sVersion, "1", resourceGroupId, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully sync changes from alibaba to rancher", func() {
			testCaseID = 294
			alibabaSyncCheck(cluster, ctx.RancherAdminClient, csClient, clusterId, resourceGroupId)
		})
	})

})
