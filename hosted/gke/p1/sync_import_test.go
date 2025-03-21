package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncImport", func() {
	for _, testData := range []struct {
		qaseID    int64
		isUpgrade bool
		testBody  func(cluster *management.Cluster, client *rancher.Client)
		testTitle string
	}{
		{
			qaseID:    58,
			isUpgrade: true,
			testBody:  syncK8sVersionUpgradeCheck,
			testTitle: "should Sync from GCE to Rancher - changed k8s version",
		},
		{
			qaseID:    61,
			isUpgrade: false,
			testBody:  syncNodepoolsCheck,
			testTitle: "should Sync from GCE to Rancher - add/delete nodepool",
		},
	} {
		testData := testData
		When("a cluster is import", func() {
			BeforeEach(func() {
				if testData.isUpgrade && helpers.SkipUpgradeTests {
					Skip(helpers.SkipUpgradeTestsLog)
				}
				k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCredID, zone, "", testData.isUpgrade)
				Expect(err).NotTo(HaveOccurred())
				GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

				err = helper.CreateGKEClusterOnGCloud(zone, clusterName, project, k8sVersion)
				Expect(err).To(BeNil())

				cluster, err = helper.ImportGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, zone, project)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})
			AfterEach(func() {
				if ctx.ClusterCleanup && cluster != nil {
					err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
					Expect(err).To(BeNil())
					err = helper.DeleteGKEClusterOnGCloud(zone, project, clusterName)
					Expect(err).To(BeNil())
				} else {
					fmt.Println("Skipping downstream cluster deletion: ", clusterName)
				}
			})

			It(testData.testTitle, func() {
				testCaseID = testData.qaseID
				testData.testBody(cluster, ctx.RancherAdminClient)
			})
		})
	}
})
