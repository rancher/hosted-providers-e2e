package support_matrix_test

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	management "github.com/rancher/rancher/tests/framework/clients/rancher/generated/management/v3"
	"github.com/rancher/rancher/tests/framework/extensions/clusters"
	"github.com/rancher/rancher/tests/framework/extensions/clusters/gke"
	nodestat "github.com/rancher/rancher/tests/framework/extensions/nodes"
	"github.com/rancher/rancher/tests/framework/extensions/workloads/pods"
	"github.com/rancher/rancher/tests/framework/pkg/config"
	namegen "github.com/rancher/rancher/tests/framework/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SupportMatrixProvisioning", func() {

	for _, version := range availableVersionList {
		version := version

		When(fmt.Sprintf("a cluster is created with k8s version %s", version), func() {
			var (
				clusterName string
				cluster     *management.Cluster
			)
			BeforeEach(func() {
				gkeConfig := new(gke.ClusterConfig)
				config.LoadAndUpdateConfig(gke.GKEClusterConfigConfigurationFileKey, gkeConfig, func() {
					gkeConfig.ProjectID = helpers.GetGKEProjectID()
					gkeConfig.Zone = helpers.GetGKEZone()
					gkeConfig.KubernetesVersion = &version
				})

				var err error
				clusterName = namegen.AppendRandomString("gkehostcluster")
				cluster, err = gke.CreateGKEHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
				Expect(err).To(BeNil())
			})
			AfterEach(func() {
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherClient)
				Expect(err).To(BeNil())
			})
			It("should successfully provision the cluster", func() {
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
