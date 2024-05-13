package k8s_chart_support_upgrade_test

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

var _ = Describe("K8sChartSupportUpgradeProvisioning", func() {

	var (
		cluster *management.Cluster
	)
	BeforeEach(func() {
		gkeConfig := new(management.GKEClusterConfigSpec)
		config.LoadAndUpdateConfig(gke.GKEClusterConfigConfigurationFileKey, gkeConfig, func() {
			gkeConfig.ProjectID = project
			gkeConfig.Zone = zone
			labels := helper.GetLabels()
			gkeConfig.Labels = &labels
			gkeConfig.KubernetesVersion = &k8sVersion
			for _, np := range gkeConfig.NodePools {
				np.Version = &k8sVersion
			}
		})

		var err error
		cluster, err = gke.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	It("should successfully test k8s chart support provisioning in an upgrade scenario", func() {
		GinkgoLogr.Info(fmt.Sprintf("Testing K8s %s chart support for provisioning on Rancher upgraded from %s to %s", helpers.K8sUpgradedMinorVersion, helpers.RancherVersion, helpers.RancherUpgradeVersion))

		testCaseID = 62 // Report to Qase
		commonChartSupportUpgrade(&ctx, cluster, clusterName, helpers.RancherUpgradeVersion, helpers.RancherHostname, helpers.K8sUpgradedMinorVersion)
	})

})
