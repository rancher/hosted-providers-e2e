/*
Copyright © 2023 - 2024 SUSE LLC

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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/norman/types/slice"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx                                       helpers.Context
	clusterName, k8sVersion, upgradeToVersion string
	testCaseID                                int64
	region                                    = helpers.GetEKSRegion()
)

func TestP1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P1 Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	helpers.CommonSynchronizedBeforeSuite()
	return nil
}, func() {
	ctx = helpers.CommonBeforeSuite()
})

var _ = BeforeEach(func() {
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

// updateClusterInUpdatingState runs checks to ensure cluster in an updating state can be updated
func updateClusterInUpdatingState(cluster *management.Cluster, client *rancher.Client) {
	var exists bool
	upgradeToVersion, err := helper.GetK8sVersion(client, false)
	Expect(err).To(BeNil())

	cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, client, false)
	Expect(err).To(BeNil())
	Expect(*cluster.EKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))

	err = clusters.WaitClusterToBeInUpgrade(client, cluster.ID)
	Expect(err).To(BeNil())

	loggingTypes := []string{"api"}
	helper.UpdateLogging(cluster, client, loggingTypes, false)
	Expect(*cluster.EKSConfig.LoggingTypes).Should(HaveExactElements(loggingTypes))

	err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
	Expect(err).To(BeNil())

	Eventually(func() bool {
		GinkgoLogr.Info("Waiting for the updated changes to appear in EKSStatus.UpstreamSpec ...")
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())

		for _, loggingType := range loggingTypes {
			exists = slice.ContainsString(*cluster.EKSStatus.UpstreamSpec.LoggingTypes, loggingType)
		}
		return exists && *cluster.EKSStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
	}, "15m", "30s").Should(BeTrue())
}

func syncK8sVersionUpgradeCheck(cluster *management.Cluster, client *rancher.Client, upgradeNodeGroup bool) {
	var err error
	GinkgoLogr.Info("Upgrading cluster to version:" + upgradeToVersion)

	By("upgrading control plane", func() {
		err = helper.UpgradeEKSClusterOnAWS(region, clusterName, upgradeToVersion)
		Expect(err).To(BeNil())

		Eventually(func() string {
			GinkgoLogr.Info("Waiting for k8s upgrade to appear in EKSStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return *cluster.EKSStatus.UpstreamSpec.KubernetesVersion
		}, tools.SetTimeout(5*time.Minute), 10*time.Second).Should(Equal(upgradeToVersion), "Failed while waiting for k8s upgrade to appear in EKSStatus.UpstreamSpec")

		if !helpers.IsImport {
			// For imported clusters, EKSConfig always has null values; so we check EKSConfig only when testing provisioned clusters
			Expect(*cluster.EKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))
			for _, ng := range cluster.EKSConfig.NodeGroups {
				Expect(*ng.Version).To(BeEquivalentTo(k8sVersion), "EKSConfig.NodePools check failed")
			}
		}
	})

	if upgradeNodeGroup {
		By("upgrading the nodegroup", func() {
			GinkgoLogr.Info("Upgrading Nodegroup's EKS version")
			for _, ng := range cluster.EKSStatus.UpstreamSpec.NodeGroups {
				err = helper.UpgradeEKSNodegroupOnAWS(region, clusterName, *ng.NodegroupName, upgradeToVersion)
				Expect(err).To(BeNil())
			}

			Eventually(func() bool {
				GinkgoLogr.Info("Waiting for the nodegroup upgrade to appear in EKSStatus.UpstreamSpec ...")
				cluster, err = client.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				for _, ng := range cluster.EKSStatus.UpstreamSpec.NodeGroups {
					if ng.Version == nil || *ng.Version != upgradeToVersion {
						return false
					}
				}
				return true
			}, tools.SetTimeout(5*time.Minute), 10*time.Second).Should(BeTrue(), "Failed while waiting for nodegroup k8s upgrade to appear in EKSStatus.UpstreamSpec")

			if !helpers.IsImport {
				// For imported clusters, EKSConfig always has null values; so we check EKSConfig only when testing provisioned clusters
				Expect(*cluster.EKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))
				for _, ng := range cluster.EKSConfig.NodeGroups {
					Expect(*ng.Version).To(BeEquivalentTo(upgradeToVersion), "EKSConfig.NodePools upgrade check failed")
				}
			}
		})
	}
}
