package k8s_chart_support_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("K8sChartSupportImport", func() {
	var cluster *management.Cluster
	BeforeEach(func() {
		csClient, err := helper.CreateAliClient(region)
		Expect(err).To(BeNil())

		alibabaClusterID, err := helper.CreateACKClusterOnAlibaba(csClient, region, clusterName, k8sVersion, "1", helpers.GetACKResourceGroupID(), helpers.GetCommonMetadataLabels())
		Expect(err).To(BeNil())

		cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, alibabaClusterID)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
	})
	AfterEach(func() {
		if ctx.ClusterCleanup && cluster != nil {
			err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			alibabaClusterID, err := helper.GetAlibabaClusterID(cluster)
			if err == nil && alibabaClusterID != "" {
				csClient, err := helper.CreateAliClient(region)
				if err == nil {
					err = helper.DeleteACKClusterOnAlibaba(csClient, alibabaClusterID)
					Expect(err).To(BeNil())
				}
			}
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	It("should successfully test k8s chart support import", func() {
		testCaseID = 264 // Report to Qase
		commonchecks(ctx.RancherAdminClient, cluster)
	})

})
