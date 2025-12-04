package p1_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	cs "github.com/alibabacloud-go/cs-20151215/v5/client"
	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	"github.com/rancher/shepherd/clients/rancher"
)

var (
	ctx                   helpers.RancherContext
	cluster               *management.Cluster
	clusterName, location string
	testCaseID            int64
	csClient              *cs.Client
	region                = helpers.GetALIRegion()
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
	csClient = helpers.GetCSClient()
})

var _ = BeforeEach(func() {
	// Setting this to nil ensures we do not use the `cluster` variable value from another test running in parallel with this one.
	cluster = nil
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	location = helpers.GetALIRegion()
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func upgradeK8sVersionFromAlibabaCheck(csClient *cs.Client, clusterId string, client *rancher.Client, k8sVersion string, upgradeToVersion string) {
	By("upgrading cluster k8s version from Alibaba", func() {
		GinkgoLogr.Info(fmt.Sprintf("Upgrading cluster on alibaba console to K8s version %s", upgradeToVersion))
		err := helper.UpgradeACKOnAlibaba(csClient, clusterId, upgradeToVersion)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("waiting for upgraded k8s version to sync on rancher..."))
			return cluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
		}, "30m", "5s").Should(BeTrue(), "Timed out while waiting for upgrade to appear in UpstreamSpec")

		for _, nodepool := range cluster.AliStatus.UpstreamSpec.NodePools {
			fmt.Println(nodepool)
			// NodePool version must remain the same
			//Expect(*nodepool.OrchestratorVersion).To(Equal(k8sVersion)) //need to check, how to fetch k8s version of node pool for alibaba
		}

		if !helpers.IsImport {
			// skip this check if the cluster is imported since the AliConfig value will not be updated
			Expect(cluster.AliConfig.KubernetesVersion).To(Equal(upgradeToVersion))
			for _, nodepool := range cluster.AliConfig.NodePools {
				fmt.Println(nodepool)
				// NodePool version must remain the same
				//Expect(*nodepool.OrchestratorVersion).To(Equal(k8sVersion))
			}
		}
	})
}
