package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Import", func() {
	var cluster *management.Cluster

	When("a cluster is imported for upgrade", func() {

		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
			Expect(err).To(BeNil())
			upgradeToVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("Using kubernetes version %s for cluster %s", k8sVersion, clusterName))
			err = helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			if ctx.ClusterCleanup {
				err := helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.DeleteEKSClusterOnAWS(region, clusterName)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("Upgrade k8s version of cluster from EKS and verify it is synced back to Rancher", func() {
			testCaseID = 114

			By("upgrading the ControlPlane & NodeGroup", func() {
				syncK8sVersionUpgradeCheck(cluster, ctx.RancherAdminClient, true)

			})
		})

		It("Sync from Rancher to AWS console after a sync from AWS console to Rancher", func() {
			testCaseID = 112

			var err error
			initialNodeCount := *cluster.EKSConfig.NodeGroups[0].DesiredSize
			By("upgrading the ControlPlane", func() {
				syncK8sVersionUpgradeCheck(cluster, ctx.RancherAdminClient, false)
			})

			By("scaling up the NodeGroup", func() {
				cluster, err = helper.ScaleNodeGroup(cluster, ctx.RancherAdminClient, initialNodeCount+1, true, true)
				Expect(err).To(BeNil())
			})

			loggingTypes := []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}
			By("Adding the LoggingTypes", func() {
				cluster, err = helper.UpdateLogging(cluster, ctx.RancherAdminClient, loggingTypes, true)
				Expect(err).To(BeNil())
			})

			// Check if the desired config is set correctly
			Expect(*cluster.EKSConfig.NodeGroups[0].DesiredSize).To(BeNumerically("==", initialNodeCount+1))
			Expect(*cluster.EKSConfig.LoggingTypes).Should(HaveExactElements(loggingTypes))
		})
	})
})
