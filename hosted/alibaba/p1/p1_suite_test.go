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
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx         helpers.RancherContext
	cluster     *management.Cluster
	clusterName string
	testCaseID  int64
	csClient    *cs.Client
	region      = helpers.GetACKRegion()
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
			clusterResp, err := helper.CheckClusterK8sVersionOnAlibaba(csClient, cluster.ID)
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
		err = helper.UpgradeACKOnAlibaba(csClient, cluster.ID, upgradeToVersion)
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
	clusterID := cluster.ID
	var err error

	if upgradeToVersion != "" {
		By("upgrading the control plane k8s version", func() {
			// Verify if the cluster is already at the target version (idempotency check)
			currentCluster, err := rancherClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			if currentCluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion {
				GinkgoLogr.Info("Cluster already at target version " + upgradeToVersion + ", skipping upgrade")
				return
			}
			err = helper.UpgradeACKOnAlibaba(csClient, clusterID, upgradeToVersion)
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
		_, err := helper.AddNodePoolOnAlibaba(csClient, npName, clusterID, nodeCount, cluster.AliStatus.UpstreamSpec.ResourceGroupID, cluster.AliStatus.UpstreamSpec.VSwitchIDs)
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
	npID, err = helper.GetNodePoolIDByName(csClient, clusterID, npName)
	Expect(err).To(BeNil())
	Expect(npID).ToNot(BeEmpty(), "Could not find ID for new nodepool")

	By("Scaling the nodepool", func() {
		const scaleCount = nodeCount + 1
		err := helper.ScaleNodePoolOnAlibaba(csClient, clusterID, scaleCount, npID)
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
			return helper.IsNodePoolActive(csClient, clusterID, npID)
		}, "10m", "10s").Should(BeTrue(), "Timed out waiting for nodepool to become active after scaling")
	})

	By("Deleting a nodepool", func() {
		err := helper.DeleteNodePoolOnAlibaba(csClient, clusterID, npID)
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
