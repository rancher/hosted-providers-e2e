package p0_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
)

var _ = Describe("P0Import", func() {
	for _, testData := range []struct {
		qaseID    int64
		isUpgrade bool
		testBody  func(cluster *management.Cluster, client *rancher.Client, clusterName string)
		testTitle string
	}{
		{
			qaseID:    234,
			isUpgrade: false,
			testBody:  p0NodesChecks,
			testTitle: "should successfully import the cluster & add, delete, scale nodepool",
		},
		{
			qaseID:    73,
			isUpgrade: true,
			testBody:  p0upgradeK8sVersionChecks,
			testTitle: "should be able to upgrade k8s version of the imported cluster",
		},
	} {
		testData := testData
		When("a cluster is created", func() {
			var clusterId string
			BeforeEach(func() {
				k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, testData.isUpgrade)
				Expect(err).To(BeNil())

				GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
				clusterId, err = helper.CreateACKClusterOnAlibaba(csClient, region, clusterName, k8sVersion, "1", resourceGroupId, helpers.GetCommonMetadataLabels())
				Expect(err).To(BeNil())

				cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})
			AfterEach(func() {
				if ctx.ClusterCleanup {
					if cluster != nil && cluster.ID != "" {
						GinkgoLogr.Info(fmt.Sprintf("Cleaning up resource cluster: %s %s", cluster.Name, cluster.ID))
						err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
						Expect(err).To(BeNil())
					}
					err := helper.DeleteACKClusteronAlibaba(csClient, clusterId)
					Expect(err).To(BeNil())
				} else {
					fmt.Println("Skipping downstream cluster deletion: ", clusterName)
				}
			})

			It(testData.testTitle, func() {
				testCaseID = testData.qaseID
				testData.testBody(cluster, ctx.RancherAdminClient, clusterName)
			})
		})
	}
})
