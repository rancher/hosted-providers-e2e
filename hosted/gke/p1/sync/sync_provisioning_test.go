package sync_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncProvisioning", func() {
	When("a cluster is created", func() {
		var cluster *management.Cluster
		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, k8sVersion, zone, project)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			if ctx.ClusterCleanup {
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("should Sync from GCE to Rancher - changed k8s version", func() {
			testCaseID = 42
			syncK8sVersionUpgradeCheck(cluster, ctx.RancherAdminClient)
		})

		It("should Sync from GCE to Rancher - add/delete nodepool", func() {
			testCaseID = 43
			syncNodepoolsCheck(cluster, ctx.RancherAdminClient)
		})
	})
})
