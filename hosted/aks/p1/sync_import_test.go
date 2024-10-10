package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncImport", func() {
	var k8sVersion string
	var cluster *management.Cluster
	BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
			err := helper.DeleteAKSClusteronAzure(clusterName)
			Expect(err).To(BeNil())

		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})
	When("a cluster is created and imported", func() {
		BeforeEach(func() {
			var err error
			err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully Add NP from Azure and then from Rancher", func() {
			testCaseID = 293
			syncAddNodePoolFromAzureAndRancher(cluster, ctx.RancherAdminClient)
		})
	})

	When("a cluster is created and imported for upgrade", func() {
		var availableUpgradeVersions []string

		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			availableUpgradeVersions, err = helper.ListAKSAvailableVersions(ctx.RancherAdminClient, cluster.ID)
			Expect(err).To(BeNil())
		})

		It("should successfully Change k8s version from Azure should change the CP k8s version and list of available version for NPs", func() {
			testCaseID = 294
			upgradeCPK8sFromAzureAndNPFromRancherCheck(cluster, ctx.RancherAdminClient, k8sVersion, availableUpgradeVersions[0])
		})

		XIt("Sync from Azure to Rancher - But edit from Rancher before the sync finishes (edit on different fields)", func() {
			// TODO: Blocked due to: https://github.com/rancher/aks-operator/issues/676
			testCaseID = 295
			syncEditDifferentFieldsCheck(cluster, ctx.RancherAdminClient, availableUpgradeVersions[0])
		})

		FIt("Upgrade same k8s version from Azure and Rancher, at the same time", func() {
			testCaseID = 297
			syncK8sUpgradeCheck(cluster, ctx.RancherAdminClient, availableUpgradeVersions[0], availableUpgradeVersions[0])
		})

		FIt("Upgrade k8s version from Azure and Upgrade higher k8s version from Rancher at the same time", func() {
			testCaseID = 298
			Expect(len(availableUpgradeVersions)).To(BeNumerically(">=", 2))
			azureUpgradeVersion := availableUpgradeVersions[0]
			rancherHigherUpgradeVersion := availableUpgradeVersions[1]
			Expect(helpers.VersionCompare(rancherHigherUpgradeVersion, azureUpgradeVersion)).To(Equal(1))
			syncK8sUpgradeCheck(cluster, ctx.RancherAdminClient, azureUpgradeVersion, rancherHigherUpgradeVersion)
		})

	})

	When("a cluster is created and imported with multiple nodepools", func() {
		BeforeEach(func() {
			var err error
			err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels(), "--nodepool-name", "systempool0")
			Expect(err).To(BeNil())

			err = helper.AddNodePoolOnAzure("systempool1", clusterName, clusterName, "2", "--mode", "System")
			Expect(err).To(BeNil())
			err = helper.AddNodePoolOnAzure("userpool0", clusterName, clusterName, "2", "--mode", "User")
			Expect(err).To(BeNil())
			err = helper.AddNodePoolOnAzure("userpool1", clusterName, clusterName, "2", "--mode", "User")
			Expect(err).To(BeNil())

			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		XIt("Delete nodepool in Azure and edit the cluster in Rancher when delete in Azure is in progress", func() {
			// Blocked by: https://github.com/rancher/aks-operator/issues/676#issuecomment-2404279109
			testCaseID = 296
			syncDeleteNPFromAzureEditFromRancher(cluster, ctx.RancherAdminClient)
		})
	})

})
