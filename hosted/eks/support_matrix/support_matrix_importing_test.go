package support_matrix_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/rancher/tests/framework/pkg/config"
	namegen "github.com/rancher/rancher/tests/framework/pkg/namegenerator"

	"fmt"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	management "github.com/rancher/rancher/tests/framework/clients/rancher/generated/management/v3"
	"github.com/rancher/rancher/tests/framework/extensions/clusters"
	nodestat "github.com/rancher/rancher/tests/framework/extensions/nodes"
	"github.com/rancher/rancher/tests/framework/extensions/workloads/pods"
)

var _ = Describe("SupportMatrixImporting", func() {

	for _, version := range availableVersionList {
		version := version

		When(fmt.Sprintf("a cluster is created with kubernetes version %s", version), func() {
			var (
				clusterName string
				cluster     *management.Cluster
				region      string
			)

			BeforeEach(func() {
				region = helpers.GetEKSRegion()
				clusterName = namegen.AppendRandomString("ekshostcluster")

				eksClusterConfig := new(helper.ImportClusterConfig)
				config.LoadAndUpdateConfig("eksClusterConfig", eksClusterConfig, func() {
					eksClusterConfig.Region = region
				})

				err := helper.CreateEKSClusterOnAWS(region, clusterName, version, "1")
				Expect(err).To(BeNil())
				cluster, err = helper.ImportEKSHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
				Expect(err).To(BeNil())
			})
			AfterEach(func() {
				err := helper.DeleteEKSHostCluster(cluster, ctx.RancherClient)
				Expect(err).To(BeNil())
				err = helper.DeleteEKSClusterOnAWS(region, clusterName)
				Expect(err).To(BeNil())
			})

			It("should successfully import the cluster", func() {
				By("checking cluster name is same", func() {
					Expect(cluster.Name).To(BeEquivalentTo(clusterName))
				})

				By("checking service account token secret", func() {
					success, err := clusters.CheckServiceAccountTokenSecret(ctx.RancherClient, clusterName)
					Expect(err).To(BeNil())
					Expect(success).To(BeTrue())
				})

				By("checking all management nodes are ready", func() {
					err := nodestat.AllManagementNodeReady(ctx.RancherClient, cluster.ID, helpers.Timeout)
					Expect(err).To(BeNil())
				})

				By("checking all pods are ready", func() {
					podErrors := pods.StatusPods(ctx.RancherClient, cluster.ID)
					Expect(podErrors).To(BeEmpty())
				})
			})
		})
	}
})
