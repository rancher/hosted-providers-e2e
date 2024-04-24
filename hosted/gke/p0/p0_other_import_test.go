package p0_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/gke"
	"github.com/rancher/shepherd/pkg/config"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P0OtherImport", func() {
	When("a cluster is created", func() {
		var cluster *management.Cluster

		BeforeEach(func() {
			var err error
			err = helper.CreateGKEClusterOnGCloud(zone, clusterName, project, k8sVersion)
			Expect(err).To(BeNil())

			gkeConfig := new(helper.ImportClusterConfig)
			config.LoadAndUpdateConfig(gke.GKEClusterConfigConfigurationFileKey, gkeConfig, func() {
				gkeConfig.ProjectID = project
				gkeConfig.Zone = zone
				labels := helper.GetLabels()
				gkeConfig.Labels = &labels
				for _, np := range gkeConfig.NodePools {
					np.Version = &k8sVersion
				}
			})
			cluster, err = helper.ImportGKEHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
			// Workaround to add new Nodegroup till https://github.com/rancher/aks-operator/issues/251 is fixed
			cluster.GKEConfig = cluster.GKEStatus.UpstreamSpec
		})
		AfterEach(func() {
			if ctx.ClusterCleanup {
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherClient)
				Expect(err).To(BeNil())
				err = helper.DeleteGKEClusterOnGCloud(zone, project, clusterName)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("should fail to reimport an imported cluster", func() {
			testCaseID = 53
			_, err := helper.ImportGKEHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("cluster already exists for GKE cluster [%s] in zone [%s]", clusterName, zone)))
		})

		It("should be able to update mutable parameter", func() {
			testCaseID = 59
			updateLoggingAndMonitoringServiceCheck(ctx, cluster)
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 61
			updateAutoScaling(ctx, cluster)
		})

		It("should be able to reimport a deleted cluster", func() {
			testCaseID = 85
			err := helper.DeleteGKEHostCluster(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
			cluster, err = helper.ImportGKEHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
		})
	})
})
