/*
Copyright © 2026-2027 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package p1_test

import (
	"fmt"
	"testing"

	cs "github.com/alibabacloud-go/cs-20151215/v5/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/pkg/config"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	ali "github.com/rancher/shepherd/extensions/clusters/alibaba"
)

var (
	ctx             helpers.RancherContext
	cluster         *management.Cluster
	clusterName     string
	testCaseID      int64
	region          = helpers.GetACKRegion()
	resourceGroupId = helpers.GetACKResourceGroupID()
	csClient        *cs.Client
)

func TestP1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alibaba P1 Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	helpers.CommonSynchronizedBeforeSuite()
	return nil
}, func() {
	ctx = helpers.CommonBeforeSuite()
	GinkgoLogr.Info(fmt.Sprintf("Running tests against Rancher Host: %s", ctx.RancherAdminClient.RancherConfig.Host))
})

var _ = BeforeEach(func() {
	cluster = nil
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
})

var _ = ReportBeforeEach(func(report SpecReport) {
	testCaseID = -1
	GinkgoLogr.Info(fmt.Sprintf("========== Starting Test: %s ==========", report.FullText()))
})

var _ = ReportAfterEach(func(report SpecReport) {
	Qase(testCaseID, report)
})

func syncK8sVersionUpgradeFromRancher(cluster *management.Cluster, csClient *cs.Client, client *rancher.Client, upgradeToVersion string) {
	GinkgoLogr.Info(fmt.Sprintf("Syncing k8s version upgrade from Rancher to Alibaba for cluster %s", cluster.Name))
	By("upgrading cluster k8s version from Rancher", func() {
		if helpers.IsImport {
			cluster.AliConfig = cluster.AliStatus.UpstreamSpec
		}
		var err error
		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, client, true, true, true)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			aliClusterID, err := helper.GetAlibabaClusterID(cluster)
			if err != nil {
				return false
			}
			clusterResp, err := helper.CheckClusterK8sVersionOnAlibaba(csClient, aliClusterID)
			Expect(err).NotTo(HaveOccurred())
			current := clusterResp
			GinkgoLogr.Info("Waiting for upgraded k8s version to sync on alibaba...")
			return current == upgradeToVersion
		}, "20m", "5s").Should(BeTrue(), "Timed out while waiting for upgrade to appear on alibaba")
	})
}

func syncK8sVersionUpgradeCheck(cluster *management.Cluster, csClient *cs.Client, client *rancher.Client, upgradeToVersion string) {
	GinkgoLogr.Info(fmt.Sprintf("Syncing k8s version upgrade from Alibaba to Rancher for cluster %s", cluster.ID))
	By("upgrading cluster k8s version from Alibaba", func() {
		currentCluster, err := client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		if currentCluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion {
			GinkgoLogr.Info("Cluster already at target version " + upgradeToVersion + ", skipping upgrade")
			return
		}
		GinkgoLogr.Info(fmt.Sprintf("Upgrading cluster on alibaba console to K8s version %s", upgradeToVersion))
		aliClusterID, err := helper.GetAlibabaClusterID(cluster)
		Expect(err).To(BeNil())
		err = helper.UpgradeACKOnAlibaba(csClient, aliClusterID, upgradeToVersion)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err := client.Management.Cluster.ByID(cluster.ID)
			if err != nil {
				return false
			}
			GinkgoLogr.Info("waiting for upgraded k8s version to sync on rancher...")
			return cluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
		}, "30m", "5s").Should(BeTrue(), "Timed out while waiting for upgrade to appear in UpstreamSpec")
	})
}

func aliNodePoolSyncCheck(cluster *management.Cluster, csClient *cs.Client, rancherClient *rancher.Client, upgradeToVersion string) {
	GinkgoLogr.Info(fmt.Sprintf("Syncing nodepool changes from Alibaba to Rancher for cluster %s", cluster.Name))
	aliClusterID, err := helper.GetAlibabaClusterID(cluster)
	Expect(err).To(BeNil())

	if upgradeToVersion != "" {
		By("upgrading the control plane k8s version", func() {
			// Verify if the cluster is already at the target version (idempotency check)
			currentCluster, err := rancherClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			if currentCluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion {
				GinkgoLogr.Info("Cluster already at target version " + upgradeToVersion + ", skipping upgrade")
				return
			}
			err = helper.UpgradeACKOnAlibaba(csClient, aliClusterID, upgradeToVersion)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err = rancherClient.Management.Cluster.ByID(cluster.ID)
				if err != nil {
					return false
				}
				return cluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
			}, "30m", "30s").Should(BeTrue(), "Timed out waiting for cluster upgrade to sync")
		})
	}

	const (
		npName          = "syncnodepool"
		nodeCount int64 = 1
	)
	currentNPCount := len(cluster.AliStatus.UpstreamSpec.NodePools)

	By("Adding a nodepool", func() {
		_, err := helper.AddNodePoolOnAlibaba(csClient, npName, aliClusterID, nodeCount, cluster.AliStatus.UpstreamSpec.ResourceGroupID, cluster.AliStatus.UpstreamSpec.VSwitchIDs)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = rancherClient.Management.Cluster.ByID(cluster.ID)
			if err != nil {
				return false
			}
			if len(cluster.AliStatus.UpstreamSpec.NodePools) == currentNPCount {
				return false
			}
			for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
				if nodepool.Name == npName {
					return true
				}
			}
			return false
		}, "10m", "10s").Should(BeTrue(), "Timed out while waiting for new nodepool to appear in UpstreamSpec")
	})

	var npID string
	npID, err = helper.GetNodePoolIDByName(csClient, aliClusterID, npName)
	Expect(err).To(BeNil())
	Expect(npID).ToNot(BeEmpty(), "Could not find ID for new nodepool")
	By("Scaling the nodepool", func() {
		const scaleCount = nodeCount + 1
		err := helper.ScaleNodePoolOnAlibaba(csClient, aliClusterID, scaleCount, npID)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = rancherClient.Management.Cluster.ByID(cluster.ID)
			if err != nil {
				return false
			}
			for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
				if nodepool.Name == npName {
					return *nodepool.DesiredSize == scaleCount
				}
			}
			return false
		}, "10m", "10s").Should(BeTrue(), "Timed out while waiting for Scale up to appear in UpstreamSpec")

		// Wait for nodepool to be active on Alibaba before proceeding
		Eventually(func() bool {
			return helper.IsNodePoolActive(csClient, aliClusterID, npID)
		}, "10m", "10s").Should(BeTrue(), "Timed out waiting for nodepool to become active after scaling")
	})

	By("Deleting a nodepool", func() {
		err := helper.DeleteNodePoolOnAlibaba(csClient, aliClusterID, npID)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = rancherClient.Management.Cluster.ByID(cluster.ID)
			if err != nil {
				return false
			}
			if len(cluster.AliStatus.UpstreamSpec.NodePools) != currentNPCount {
				return false
			}
			for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
				if nodepool.Name == npName {
					return false
				}
			}
			return true
		}, "10m", "10s").Should(BeTrue(), "Timed out while waiting for nodepool deletion to appear in UpstreamSpec")
	})
}

func upgradeAlibabaClusterAndNodePool(cluster *management.Cluster, client *rancher.Client, upgradeToVersion string) {
	var err error
	originalLen := len(cluster.AliConfig.NodePools)
	newNodePoolName := namegen.AppendRandomString("np")
	GinkgoLogr.Info("Upgrading control plane to version:" + upgradeToVersion)

	By("upgrading the ControlPlane", func() {
		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient, false, true, true)
		Expect(err).To(BeNil())
	})

	var aliClusterConfig ali.ClusterConfig
	config.LoadConfig(ali.ALIClusterConfigConfigurationFileKey, &aliClusterConfig)

	updateFunc := func(cluster *management.Cluster) {
		var updatedNodePoolsList = make([]management.AliNodePool, 0)
		nodePools := ali.MapAliNodePoolsFromAliNodePool(aliClusterConfig.NodePools)
		newNodePool := nodePools[0]
		newNodePool.Name = newNodePoolName
		updatedNodePoolsList = append(updatedNodePoolsList, newNodePool)
		cluster.AliConfig.NodePools = updatedNodePoolsList
	}

	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	Expect(len(cluster.AliConfig.NodePools)).To(BeEquivalentTo(originalLen))
	for _, np := range cluster.AliConfig.NodePools {
		Expect(np.Name).To(Equal(newNodePoolName))
	}

	err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
	Expect(err).To(BeNil())

	// wait until the update is visible on the cluster
	Eventually(func() bool {
		GinkgoLogr.Info("Waiting for the new nodepool to appear in AliStatus.UpstreamSpec ...")
		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		for _, np := range cluster.AliStatus.UpstreamSpec.NodePools {
			if np.Name != newNodePoolName {
				return false
			}
		}
		return true
	}, "5m", "15s").Should(BeTrue())
}

func invalidCloudCredentialsCheck(cluster *management.Cluster, client *rancher.Client, cloudCredID string) {
	currentCC, err := client.Management.CloudCredential.ByID(cloudCredID)
	Expect(err).To(BeNil())
	err = client.Management.CloudCredential.Delete(currentCC)
	Expect(err).To(BeNil())
	GinkgoLogr.Info(fmt.Sprintf("Deleting existing Cloud Credentials: %s:%s", currentCC.Name, currentCC.ID))
	const scaleCount int64 = 2
	cluster, err = helper.ScaleNodePool(cluster, client, scaleCount, false, false)
	Expect(err).To(BeNil())
	Eventually(func() string {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		return cluster.Transitioning
	}, "3m", "2s").Should(Equal("error"), "Timed out waiting for cluster to transition into error")

	// Create new cloud credentials and update the cluster config with it
	newCCID, err := helpers.CreateCloudCredentials(client)
	Expect(err).To(BeNil())

	cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, "", client, false, true, false)

	Expect(err).To(BeNil())
	Expect(cluster.AliConfig.AlibabaCredentialSecret).To(Equal(newCCID))
	err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
	Expect(err).To(BeNil())
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		return cluster.AliStatus.UpstreamSpec.AlibabaCredentialSecret == newCCID
	}, "5m", "5s").Should(BeTrue())

	for _, nodepool := range cluster.AliConfig.NodePools {
		Expect(nodepool.DesiredSize).To(Equal(scaleCount))
	}

	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
			if *nodepool.DesiredSize != scaleCount {
				return false
			}
		}
		return true
	}, "5m", "5s").Should(BeTrue(), "Timed out waiting for upstream spec to reflect node count")

	// Update the context so that any future tests are not disrupted
	GinkgoLogr.Info(fmt.Sprintf("Updating the new Cloud Credentials %s to the context", newCCID))
	ctx.CloudCredID = newCCID
}

func updateCloudCredentialsCheck(cluster *management.Cluster, client *rancher.Client) {

	newCCID, err := helpers.CreateCloudCredentials(client)
	GinkgoLogr.Info("Updating cloud credentials to ID:" + newCCID)
	Expect(err).To(BeNil())
	updateFunc := func(cluster *management.Cluster) {
		cluster.AliConfig.AlibabaCredentialSecret = newCCID
		// Preserve KubernetesVersion to avoid empty string error
		if cluster.AliStatus != nil && cluster.AliStatus.UpstreamSpec != nil {
			cluster.AliConfig.KubernetesVersion = cluster.AliStatus.UpstreamSpec.KubernetesVersion
		}
	}
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	Expect(cluster.AliConfig.AlibabaCredentialSecret).To(Equal(newCCID))
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		return cluster.AliStatus.UpstreamSpec.AlibabaCredentialSecret == newCCID
	}, "5m", "5s").Should(BeTrue(), "Failed while upstream cloud credentials update")

	cluster, err = helper.AddNodePool(cluster, client, 1, "", true, true)
	Expect(err).To(BeNil())
}

func updateAutoScaling(cluster *management.Cluster, client *rancher.Client) {
	By("enabling autoscaling with custom minCount and maxCount", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, client, 0, "", true, true)
		Expect(err).To(BeNil())
	})

	By("disabling autoscaling", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, client, 0, "", true, true)
		Expect(err).To(BeNil())
	})
}
