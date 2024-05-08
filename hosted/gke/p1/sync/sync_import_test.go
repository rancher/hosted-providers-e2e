package sync_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncImport", func() {
	When("a cluster is imported", func() {
		var cluster *management.Cluster

		BeforeEach(func() {
			err := helper.CreateGKEClusterOnGCloud(zone, clusterName, project, k8sVersion)
			Expect(err).To(BeNil())

			cluster, err = helper.ImportGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, zone, project)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			// Workaround to add new Nodegroup till https://github.com/rancher/aks-operator/issues/251 is fixed
			cluster.GKEConfig = cluster.GKEStatus.UpstreamSpec
		})
		AfterEach(func() {
			if ctx.ClusterCleanup {
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.DeleteGKEClusterOnGCloud(zone, project, clusterName)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("should Sync from GCE to Rancher - changed k8s version", func() {
			testCaseID = 86
			syncK8sVersionUpgradeCheck(cluster, ctx.RancherAdminClient)
		})

		It("should Sync from GCE to Rancher - add/delete nodepool", func() {
			testCaseID = 341
			syncNodepoolsCheck(cluster, ctx.RancherAdminClient)
		})
	})
})
