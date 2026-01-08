/*
Copyright © 2025 SUSE LLC

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
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	cs "github.com/alibabacloud-go/cs-20151215/v3/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx         helpers.RancherContext
	cluster     *management.Cluster
	clusterName string
	testCaseID  int64
	region      string
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

func syncK8sVersionUpgradeCheck(cluster *management.Cluster, client *rancher.Client, upgradeNodeGroup bool, currentVersion, upgradeVersion string) {
	GinkgoLogr.Info("Upgrading cluster to version:" + upgradeVersion)
	By("upgrading the ControlPlane and NodePools in Alibaba", func() {
		// Verify if the cluster is already at the target version (idempotency check)
		currentCluster, err := client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		if currentCluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeVersion {
			GinkgoLogr.Info("Cluster already at target version " + upgradeVersion + ", skipping upgrade")
			return
		}

		csClient, err := createAliClient(cluster.AliStatus.UpstreamSpec.RegionID)
		Expect(err).To(BeNil())
		err = UpgradeACKOnAlibaba(csClient, cluster.AliStatus.UpstreamSpec.ClusterID, upgradeVersion)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err := client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			GinkgoLogr.Info("waiting for upgraded k8s version to sync on rancher...")
			return cluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeVersion
		}, "30m", "5s").Should(BeTrue(), "Timed out while waiting for upgrade to appear in UpstreamSpec")
	})

	for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
		fmt.Println(nodepool)
		// NodePool version must remain the same
		//Expect(*nodepool.OrchestratorVersion).To(Equal(k8sVersion)) //need to check, how to fetch k8s version of node pool for alibaba
	}
}

func createAliClient(region string) (*cs.Client, error) {
	config := &openapi.Config{
		AccessKeyId:     tea.String(os.Getenv("ALIBABA_ACCESS_KEY_ID")),
		AccessKeySecret: tea.String(os.Getenv("ALIBABA_ACCESS_KEY_SECRET")),
	}
	config.Endpoint = tea.String(fmt.Sprintf("cs.%s.aliyuncs.com", region))
	return cs.NewClient(config)
}

func UpgradeACKOnAlibaba(csClient *cs.Client, clusterId string, upgradeToVersion string, additionalArgs ...string) error {
	upgradeClusterRequest := &cs.UpgradeClusterRequest{
		NextVersion: tea.String(upgradeToVersion),
	}
	runtime := &util.RuntimeOptions{}
	headers := make(map[string]*string)

	_, err := csClient.UpgradeClusterWithOptions(tea.String(clusterId), upgradeClusterRequest, headers, runtime)
	if err != nil {
		return fmt.Errorf("cluster upgrade failed: %w", err)
	}
	return nil
}

func aliSyncCheck(cluster *management.Cluster, client *rancher.Client, upgradeToVersion string) {
	csClient, err := createAliClient(cluster.AliStatus.UpstreamSpec.RegionID)
	Expect(err).To(BeNil())
	clusterID := cluster.AliStatus.UpstreamSpec.ClusterID

	By("upgrading the control plane k8s version", func() {
		// Verify if the cluster is already at the target version (idempotency check)
		currentCluster, err := client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		if currentCluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion {
			GinkgoLogr.Info("Cluster already at target version " + upgradeToVersion + ", skipping upgrade")
			return
		}

		err = UpgradeACKOnAlibaba(csClient, clusterID, upgradeToVersion)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			return cluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
		}, "30m", "30s").Should(BeTrue(), "Timed out waiting for cluster upgrade to sync")
	})

	const (
		npName          = "syncnodepool"
		nodeCount int64 = 1
	)
	currentNPCount := len(cluster.AliStatus.UpstreamSpec.NodePools)

	By("Adding a nodepool", func() {
		err := CreateACKNodePoolOnAlibaba(csClient, clusterID, npName)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
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
	resp, err := csClient.DescribeClusterNodePoolsWithOptions(tea.String(clusterID), make(map[string]*string), &util.RuntimeOptions{})
	Expect(err).To(BeNil())
	for _, np := range resp.Body.Nodepools {
		if tea.StringValue(np.NodepoolInfo.Name) == npName {
			npID = tea.StringValue(np.NodepoolInfo.NodepoolId)
			break
		}
	}
	Expect(npID).ToNot(BeEmpty(), "Could not find ID for new nodepool")

	By("Scaling the nodepool", func() {
		const scaleCount = nodeCount + 1
		err := ScaleACKNodePoolOnAlibaba(csClient, clusterID, npID, scaleCount)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
				if nodepool.Name == npName {
					return *nodepool.DesiredSize == scaleCount
				}
			}
			return false
		}, "10m", "10s").Should(BeTrue(), "Timed out while waiting for Scale up to appear in UpstreamSpec")

		// Wait for nodepool to be active on Alibaba before proceeding
		Eventually(func() bool {
			return isNodePoolActive(csClient, clusterID, npID)
		}, "10m", "10s").Should(BeTrue(), "Timed out waiting for nodepool to become active after scaling")
	})

	By("Deleting a nodepool", func() {
		err := DeleteACKNodePoolOnAlibaba(csClient, clusterID, npID)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
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

func CreateACKNodePoolOnAlibaba(csClient *cs.Client, clusterId, newPoolName string) error {
	resp, err := csClient.DescribeClusterNodePoolsWithOptions(tea.String(clusterId), make(map[string]*string), &util.RuntimeOptions{})
	if err != nil {
		return err
	}
	if len(resp.Body.Nodepools) == 0 {
		return fmt.Errorf("no existing nodepools to copy config from")
	}
	template := resp.Body.Nodepools[0]

	req := &cs.CreateClusterNodePoolRequest{
		NodepoolInfo: &cs.CreateClusterNodePoolRequestNodepoolInfo{
			Name: tea.String(newPoolName),
		},
		ScalingGroup: &cs.CreateClusterNodePoolRequestScalingGroup{
			VswitchIds:         template.ScalingGroup.VswitchIds,
			InstanceTypes:      template.ScalingGroup.InstanceTypes,
			SystemDiskCategory: template.ScalingGroup.SystemDiskCategory,
			SystemDiskSize:     template.ScalingGroup.SystemDiskSize,
			DesiredSize:        tea.Int64(2),
		},
		KubernetesConfig: &cs.CreateClusterNodePoolRequestKubernetesConfig{
			Runtime:        template.KubernetesConfig.Runtime,
			RuntimeVersion: template.KubernetesConfig.RuntimeVersion,
		},
	}
	if template.TeeConfig != nil {
		req.TeeConfig = &cs.CreateClusterNodePoolRequestTeeConfig{
			TeeEnable: template.TeeConfig.TeeEnable,
		}
	}

	_, err = csClient.CreateClusterNodePoolWithOptions(tea.String(clusterId), req, make(map[string]*string), &util.RuntimeOptions{})
	return err
}

func ScaleACKNodePoolOnAlibaba(csClient *cs.Client, clusterId, nodepoolId string, count int64) error {
	req := &cs.ModifyClusterNodePoolRequest{
		ScalingGroup: &cs.ModifyClusterNodePoolRequestScalingGroup{
			DesiredSize: tea.Int64(count),
		},
	}
	_, err := csClient.ModifyClusterNodePoolWithOptions(tea.String(clusterId), tea.String(nodepoolId), req, make(map[string]*string), &util.RuntimeOptions{})
	return err
}

func DeleteACKNodePoolOnAlibaba(csClient *cs.Client, clusterId, nodepoolId string) error {
	_, err := csClient.DeleteClusterNodepoolWithOptions(tea.String(clusterId), tea.String(nodepoolId), &cs.DeleteClusterNodepoolRequest{
		Force: tea.Bool(true),
	}, make(map[string]*string), &util.RuntimeOptions{})
	return err
}

func isNodePoolActive(csClient *cs.Client, clusterId, nodepoolId string) bool {
	resp, err := csClient.DescribeClusterNodePoolDetailWithOptions(tea.String(clusterId), tea.String(nodepoolId), make(map[string]*string), &util.RuntimeOptions{})
	if err != nil {
		GinkgoLogr.Info(fmt.Sprintf("Error checking nodepool status: %v", err))
		return false
	}
	status := tea.StringValue(resp.Body.Status.State)
	GinkgoLogr.Info(fmt.Sprintf("Nodepool %s status: %s", nodepoolId, status))
	return status == "active"
}
