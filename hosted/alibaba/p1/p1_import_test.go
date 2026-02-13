package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Import", func() {

	var k8sVersion string
	var err error

	BeforeEach(func() {

		cluster = nil
		GinkgoLogr.Info(fmt.Sprintf("Running on process: %d", GinkgoParallelProcess()))
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
		csClient, err = helper.CreateAliClient(region)
		Expect(err).To(BeNil())

	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				GinkgoLogr.Info(fmt.Sprintf("Cleaning up resource cluster: %s %s", cluster.Name, cluster.ID))
				err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).NotTo(HaveOccurred())
			}
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}

	})
	When("Cluster is Created & Imported", func() {
		var clusterId string
		BeforeEach(func() {
			var err error
			clusterId, err = helper.CreateACKClusterOnAlibaba(csClient, region, clusterName, k8sVersion, "1", resourceGroupId, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully update with new cloud credentials", func() {
			testCaseID = -1
			updateCloudCredentialsCheck(cluster, ctx.RancherAdminClient)
		})

		It("should fail to update with invalid (deleted) cloud credential and update when the cloud credentials becomes valid", func() {
			testCaseID = -1
			invalidCloudCredentialsCheck(cluster, ctx.RancherAdminClient, ctx.CloudCredID)
		})

		It("should be able to update autoscaling", func() {
			testCaseID = -1
			updateAutoScaling(cluster, ctx.RancherAdminClient)
		})

		It("should fail to reimport an imported cluster", func() {
			testCaseID = -1
			_, err := helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
			Expect(err).To(HaveOccurred())
		})

		It("should be possible to re-import a deleted cluster", func() {
			testCaseID = -1
			err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			clusterID := cluster.ID
			Eventually(func() error {
				_, err := ctx.RancherAdminClient.Management.Cluster.ByID(clusterID)
				return err
			}, "30s", "3s").ShouldNot(BeNil())
			cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})
	})

})
