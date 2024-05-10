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

package p0_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

const (
	increaseBy = 1
)

var (
	ctx         helpers.Context
	clusterName string
	testCaseID  int64
	location    = helpers.GetAKSLocation()
)

func TestP0(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P0 Suite")
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

func p0upgradeK8sVersionCheck(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	currentVersion := cluster.AKSConfig.KubernetesVersion
	versions, err := helper.ListAKSAvailableVersions(ctx.RancherAdminClient, cluster.ID)
	Expect(err).To(BeNil())
	Expect(versions).ToNot(BeEmpty())
	upgradeToVersion := &versions[0]
	GinkgoLogr.Info(fmt.Sprintf("Upgrading cluster to AKS version %s", *upgradeToVersion))

	By("upgrading the ControlPlane", func() {
		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		Expect(cluster.AKSConfig.KubernetesVersion).To(BeEquivalentTo(upgradeToVersion))
		for _, np := range cluster.AKSConfig.NodePools {
			Expect(np.OrchestratorVersion).To(BeEquivalentTo(currentVersion))
		}
	})

	By("upgrading the NodePools", func() {
		cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())
		Expect(cluster.AKSConfig.KubernetesVersion).To(BeEquivalentTo(upgradeToVersion))
		for _, np := range cluster.AKSConfig.NodePools {
			Expect(np.OrchestratorVersion).To(BeEquivalentTo(upgradeToVersion))
		}
	})
}

func p0NodesChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {

	helpers.ClusterIsReadyChecks(cluster, client, clusterName)

	currentNodePoolNumber := len(cluster.AKSConfig.NodePools)
	initialNodeCount := *cluster.AKSConfig.NodePools[0].Count

	By("adding a nodepool", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, increaseBy, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())
		Expect(len(cluster.AKSConfig.NodePools)).To(BeNumerically("==", currentNodePoolNumber+1))
	})
	By("deleting the nodepool", func() {
		var err error
		cluster, err = helper.DeleteNodePool(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())
		Expect(len(cluster.AKSConfig.NodePools)).To(BeNumerically("==", currentNodePoolNumber))
	})

	By("scaling up the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, ctx.RancherAdminClient, initialNodeCount+1)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())
		for i := range cluster.AKSConfig.NodePools {
			Expect(*cluster.AKSConfig.NodePools[i].Count).To(BeNumerically("==", initialNodeCount+1))
		}
	})

	By("scaling down the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, ctx.RancherAdminClient, initialNodeCount)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())
		for i := range cluster.AKSConfig.NodePools {
			Expect(*cluster.AKSConfig.NodePools[i].Count).To(BeNumerically("==", initialNodeCount))
		}
	})
}
