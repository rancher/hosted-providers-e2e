// Update k8s version of cluster for provisioning and add node groups
package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Provisioning", func() {
	var k8sVersion string
	var upgradeToVersion string
	var _ = BeforeEach(func() {

		cluster = nil
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				GinkgoLogr.Info(fmt.Sprintf("Cleaning up resource cluster: %s %s", cluster.Name, cluster.ID))
				err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})
	Context("Upgrade testing", func() {

		BeforeEach(func() {

			var err error
			upgradeToVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
		})

		When("a cluster is created", func() {

			BeforeEach(func() {
				var err error
				cluster, err = helper.CreateAlibabaHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})

			It("Update Cluster And Add Node Pool", func() {
				testCaseID = -1
				upgradeAlibabaClusterAndNodePool(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})
		})

	})
})
