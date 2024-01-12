package support_matrix_test

import (
	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	management "github.com/rancher/rancher/tests/framework/clients/rancher/generated/management/v3"
	"github.com/rancher/rancher/tests/framework/pkg/config"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	availableVersionList []string
	ctx                  helpers.Context
	project              = os.Getenv("GKE_PROJECT_ID")
	zone                 = helpers.GetGKEZone()
)

func TestSupportMatrix(t *testing.T) {
	RegisterFailHandler(Fail)
	var err error
	// TODO: Fix this behavior; move to BeforeSuite.
	ctx, err = helpers.CommonBeforeSuite("gke")
	Expect(err).To(BeNil())
	availableVersionList, err = helper.ListSingleVariantGKEAvailableVersions(ctx.RancherClient, project, ctx.CloudCred.ID, zone, "")
	Expect(err).To(BeNil())
	RunSpecs(t, "SupportMatrix Suite")
}

var _ = BeforeEach(func() {
	// re-update the config file with tags, Support Matrix suite is defined such that "testfilename" does not get updated in CommonBeforeSuite.
	gkeClusterConfig := new(management.GKEClusterConfigSpec)
	config.LoadAndUpdateConfig(helpers.GKEClusterConfigKey, gkeClusterConfig, func() {
		providerTags := helper.GetLabels()
		gkeClusterConfig.Labels = &providerTags
	})

})
