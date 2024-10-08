package p1_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/aks"
	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncProvisioning", func() {
	var cluster *management.Cluster
	var k8sVersion string
	BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
	})
	AfterEach(func() {
		if ctx.ClusterCleanup && (cluster != nil && cluster.ID != "") {
			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	When("a cluster is created", func() {
		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully Add NP from Azure and then from Rancher", func() {
			testCaseID = 224
			syncAddNodePoolFromAzureAndRancher(cluster, ctx.RancherAdminClient)
		})
	})

	When("a cluster is created for upgrade", func() {
		var availableUpgradeVersions []string

		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).NotTo(HaveOccurred())

			availableUpgradeVersions, err = helper.ListAKSAvailableVersions(ctx.RancherAdminClient, cluster.ID)
			Expect(err).To(BeNil())
		})

		It("should successfully Change k8s version from Azure should change the CP k8s version and list of available version for NPs", func() {
			testCaseID = 225
			upgradeCPK8sFromAzureAndNPFromRancherCheck(cluster, ctx.RancherAdminClient, k8sVersion, availableUpgradeVersions[0])
		})

		XIt("Sync from Azure to Rancher - But edit from Rancher before the sync finishes (edit on different fields)", func() {
			// TODO: Blocked due to: https://github.com/rancher/aks-operator/issues/676
			testCaseID = 226
			syncEditDifferentFieldsCheck(cluster, ctx.RancherAdminClient, availableUpgradeVersions[0])
		})

		FIt("Upgrade same k8s version from Azure and Rancher, at the same time", func() {
			testCaseID = 228
			syncK8sUpgradeCheck(cluster, ctx.RancherAdminClient, availableUpgradeVersions[0], availableUpgradeVersions[0])
		})

		FIt("Upgrade k8s version from Azure and Upgrade higher k8s version from Rancher at the same time", func() {
			testCaseID = 229
			Expect(len(availableUpgradeVersions)).To(BeNumerically(">=", 2))
			azureUpgradeVersion := availableUpgradeVersions[0]
			rancherHigherUpgradeVersion := availableUpgradeVersions[1]
			syncK8sUpgradeCheck(cluster, ctx.RancherAdminClient, azureUpgradeVersion, rancherHigherUpgradeVersion)
		})
	})

	When("a cluster is created with multiple nodepools", func() {
		BeforeEach(func() {
			updateFunc := func(aksConfig *aks.ClusterConfig) {
				nodepools := *aksConfig.NodePools
				npTemplate := nodepools[0]
				var updatedNodePools []aks.NodePool
				for i := 0; i < 2; i++ {
					for _, mode := range []string{"User", "System"} {
						updatedNodePools = append(updatedNodePools, aks.NodePool{
							AvailabilityZones:   npTemplate.AvailabilityZones,
							EnableAutoScaling:   npTemplate.EnableAutoScaling,
							MaxPods:             npTemplate.MaxPods,
							MaxCount:            npTemplate.MaxCount,
							MinCount:            npTemplate.MinCount,
							Mode:                mode,
							Name:                pointer.String(fmt.Sprintf("%spool%d", strings.ToLower(mode), i)),
							NodeCount:           npTemplate.NodeCount,
							OrchestratorVersion: pointer.String(k8sVersion),
							OsDiskSizeGB:        npTemplate.OsDiskSizeGB,
							OsDiskType:          npTemplate.OsDiskType,
							OsType:              npTemplate.OsType,
							VMSize:              npTemplate.VMSize,
						})
					}
				}
				aksConfig.NodePools = &updatedNodePools
			}
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		})

		FIt("Delete nodepool in Azure and edit the cluster in Rancher when delete in Azure is in progress", func() {
			testCaseID = 227
			syncDeleteNPFromAzureEditFromRancher(cluster, ctx.RancherAdminClient)
		})
	})
})
