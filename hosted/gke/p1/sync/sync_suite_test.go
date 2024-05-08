package sync_test

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx                     helpers.Context
	clusterName, k8sVersion string
	testCaseID              int64
	zone                    = helpers.GetGKEZone()
	project                 = helpers.GetGKEProjectID()
)

func TestSync(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sync Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	helpers.CommonSynchronizedBeforeSuite()
	return nil
}, func() {
	ctx = helpers.CommonBeforeSuite()
})

var _ = BeforeEach(func() {
	var err error
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCred.ID, zone, "", false)
	Expect(err).To(BeNil())
	GinkgoLogr.Info(fmt.Sprintf("Using GKE version %s", k8sVersion))
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func syncK8sVersionUpgradeCheck(cluster *management.Cluster, client *rancher.Client) {
	availableVersions, err := helper.ListGKEAvailableVersions(client, cluster.ID)
	Expect(err).To(BeNil())
	upgradeToVersion := availableVersions[0]

	By("upgrading control plane", func() {
		currentVersion := cluster.GKEConfig.KubernetesVersion

		err = helper.UpgradeGKEClusterOnGCloud(zone, clusterName, project, upgradeToVersion, false, "")
		Expect(err).To(BeNil())
		// The cluster errors out and becomes unavailable at some point due to the upgrade , so we wait until the cluster is ready
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, client)
		Expect(err).To(BeNil())
		Eventually(func() string {
			GinkgoLogr.Info("Waiting for k8s upgrade to appear in GKEStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return *cluster.GKEStatus.UpstreamSpec.KubernetesVersion
		}, tools.SetTimeout(5*time.Minute), 10*time.Second).Should(Equal(upgradeToVersion))

		// Ensure nodepool version is still the same.
		for _, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
			Expect(np.Version).To(BeEquivalentTo(currentVersion))
		}

		if !helpers.IsImport {
			// For imported clusters, GKEConfig always has null values; so we check GKEConfig only when testing provisioned clusters
			Expect(*cluster.GKEConfig.KubernetesVersion).To(Equal(upgradeToVersion))
			for _, np := range cluster.GKEConfig.NodePools {
				Expect(np.Version).To(BeEquivalentTo(currentVersion))
			}
		}
	})

	By("upgrading the node pool", func() {
		for _, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
			err = helper.UpgradeGKEClusterOnGCloud(zone, clusterName, project, upgradeToVersion, true, *np.Name)
			Expect(err).To(BeNil())
		}

		Eventually(func() bool {
			GinkgoLogr.Info("Waiting for the nodepool upgrade to appear in GKEStatus.UpstreamSpec ...")

			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			for _, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
				if *np.Version != upgradeToVersion {
					return false
				}
			}
			return true
		}, tools.SetTimeout(5*time.Minute), 10*time.Second).Should(BeTrue())

		if !helpers.IsImport {
			// For imported clusters, GKEConfig always has null values; so we check GKEConfig only when testing provisioned clusters
			for _, np := range cluster.GKEConfig.NodePools {
				Expect(*np.Version).To(BeEquivalentTo(upgradeToVersion))
			}
		}
	})
}

func syncNodepoolsCheck(cluster *management.Cluster, client *rancher.Client) {

	const poolName = "new-node-pool"
	currentNodeCount := len(cluster.GKEConfig.NodePools)

	By("adding a nodepool", func() {
		err := helper.AddNodePoolOnGCloud(zone, project, clusterName, poolName)
		Expect(err).To(BeNil())

		// The cluster does not go into updating state, so we simply wait until the number of nodepools increases
		Eventually(func() int {
			GinkgoLogr.Info("Waiting for the total nodepool count to increase in GKEStatus.UpstreamSpec ...")

			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.GKEStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(3*time.Minute), 2*time.Second).Should(Equal(currentNodeCount + 1))

		// check that the new node pool has been added
		Expect(func() bool {
			for _, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
				if *np.Name == poolName {
					return true
				}
			}
			return false

		}()).To(BeTrue())

		if !helpers.IsImport {
			// For imported clusters, GKEConfig always has null values; so we check GKEConfig only when testing provisioned clusters
			Expect(len(cluster.GKEConfig.NodePools)).To(Equal(currentNodeCount + 1))

			Expect(func() bool {
				for _, np := range cluster.GKEConfig.NodePools {
					if *np.Name == poolName {
						return true
					}
				}
				return false
			}()).To(BeTrue())
		}
	})

	By("deleting the nodepool", func() {
		err := helper.DeleteNodePoolOnGCloud(zone, project, clusterName, poolName)
		Expect(err).To(BeNil())

		// The cluster does not go into updating state, so we simply wait until the number of nodepools decreases
		Eventually(func() int {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.GKEStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(3*time.Minute), 2*time.Second).Should(Equal(currentNodeCount))

		// check that the new node pool has been deleted
		Expect(func() bool {
			for _, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
				if *np.Name == poolName {
					return true
				}
			}
			return false

		}()).To(BeFalse())

		if !helpers.IsImport {
			// For imported clusters, GKEConfig always has null values; so we check GKEConfig only when testing provisioned clusters
			Expect(len(cluster.GKEConfig.NodePools)).To(Equal(currentNodeCount))

			Expect(func() bool {
				for _, np := range cluster.GKEConfig.NodePools {
					if *np.Name == poolName {
						return true
					}
				}
				return false
			}()).To(BeFalse())
		}
	})
}
