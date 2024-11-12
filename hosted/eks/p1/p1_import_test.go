package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Import", func() {
	var cluster *management.Cluster

	BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("Using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup && (cluster != nil && cluster.ID != "") {
			err := helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			err = helper.DeleteEKSClusterOnAWS(region, clusterName)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	When("a cluster is imported for upgrade", func() {

		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("Using kubernetes version %s for cluster %s", k8sVersion, clusterName))

			err = helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("Upgrade version of node group only", func() {
			testCaseID = 88
			upgradeNodeKubernetesVersionGTCPCheck(cluster, ctx.RancherAdminClient)
		})

		// eks-operator/issues/752
		XIt("should successfully update a cluster while it is still in updating state", func() {
			testCaseID = 104
			updateClusterInUpdatingState(cluster, ctx.RancherAdminClient)
		})

		It("Update k8s version of cluster and add node groups", func() {
			testCaseID = 90
			upgradeCPAndAddNgCheck(cluster, ctx.RancherAdminClient)
		})
	})

	FIt("should successfully Import cluster with ONLY control plane", func() {
		testCaseID = 94
		err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels(), "--without-nodegroup")
		Expect(err).To(BeNil())
		cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return cluster.Transitioning == "error" && cluster.TransitioningMessage == "Cluster must have at least one managed nodegroup or one self-managed node."
		}, "5m", "2s").Should(BeTrue())
		cluster.EKSConfig = cluster.EKSStatus.UpstreamSpec
		cluster, err = helper.AddNodeGroup(cluster, 1, ctx.RancherAdminClient, false, false)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	FIt("successfully import EKS cluster with self-managed nodes", func() {
		testCaseID = 107
		err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels(), "--managed=false")
		Expect(err).To(BeNil())
		cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
	})

	FWhen("a cluster with multiple nodegroups is imported", func() {
		BeforeEach(func() {
			err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			for i := 0; i < 2; i++ {
				err = helper.AddNodeGroupOnAWS(namegen.AppendRandomString("ng"), clusterName, region)
				Expect(err).To(BeNil())
			}
			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully Import cluster with at least 2 nodegroups", func() {
			testCaseID = 105
			helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		})
	})

	When("a cluster is imported", func() {

		var _ = BeforeEach(func() {
			err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("Delete & re-import cluster", func() {
			testCaseID = 106

			var err error
			err = helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			Eventually(func() string {
				cluster, _ = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				return cluster.ID
			}, "30s", "3s").Should(BeEmpty())

			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("Update cluster logging types", func() {
			testCaseID = 77

			var err error
			loggingTypes := []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}
			By("Adding the LoggingTypes", func() {
				cluster, err = helper.UpdateLogging(cluster, ctx.RancherAdminClient, loggingTypes, true)
				Expect(err).To(BeNil())
			})

			By("Removing the LoggingTypes", func() {
				cluster, err = helper.UpdateLogging(cluster, ctx.RancherAdminClient, []string{loggingTypes[0]}, true)
				Expect(err).To(BeNil())
			})
		})

		FIt("Update Tags and Labels", func() {
			testCaseID = 81
			updateTagsAndLabels(cluster, ctx.RancherAdminClient)
		})

		Context("Reimporting/Editing a cluster with invalid config", func() {
			It("Reimport a cluster to Rancher should fail", func() {
				testCaseID = 101

				// We do not assign the cluster returned by import function to `cluster` since it will be nil and the cluster won't be deleted in AfterEach
				_, err := helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("cluster already exists for EKS cluster")))
			})

			It("Add node groups to the control-plane only cluster", func() {
				testCaseID = 95

				var err error
				err = helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.ModifyEKSNodegroupOnAWS(region, clusterName, "ranchernodes", "delete", "--wait")
				Expect(err).To(BeNil())

				cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
				Expect(err).To(BeNil())
				Eventually(func() bool {
					cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
					Expect(err).To(BeNil())
					return cluster.State == "waiting"
				}, "5m", "15s").Should(BeTrue())

				By("adding a NodeGroup", func() {
					cluster, err = helper.AddNodeGroup(cluster, 1, ctx.RancherAdminClient, false, true)
					Expect(err).To(BeNil())
				})
			})

			It("Fail to update both Public/Private access as false and invalid values of the access", func() {
				testCaseID = 103 // also covers 102
				invalidEndpointCheck(cluster, ctx.RancherAdminClient)
				invalidAccessValuesCheck(cluster, ctx.RancherAdminClient)
			})
		})
	})
})
