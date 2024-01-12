package support_matrix_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	management "github.com/rancher/rancher/tests/framework/clients/rancher/generated/management/v3"
	"github.com/rancher/rancher/tests/framework/pkg/config"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	"github.com/rancher/rancher/tests/framework/extensions/clusters/kubernetesversions"
)

var (
	availableVersionList []string
	ctx                  helpers.Context
	region               = helpers.GetEKSRegion()
)

func TestSupportMatrix(t *testing.T) {
	RegisterFailHandler(Fail)
	var err error
	// TODO: Fix this behavior; move to BeforeSuite.
	ctx, err = helpers.CommonBeforeSuite("eks")
	Expect(err).To(BeNil())
	availableVersionList, err = kubernetesversions.ListEKSAllVersions(ctx.RancherClient)
	Expect(err).To(BeNil())
	Expect(availableVersionList).ToNot(BeEmpty())
	RunSpecs(t, "SupportMatrix Suite")
}

var _ = BeforeEach(func() {
	eksClusterConfig := new(management.EKSClusterConfigSpec)
	// re-update the config file with tags, Support Matrix suite is defined such that "testfilename" does not get updated in CommonBeforeSuite.
	config.LoadAndUpdateConfig(helpers.EKSClusterConfigKey, eksClusterConfig, func() {
		tags := helper.GetTags()
		eksClusterConfig.Tags = &tags
	})
})
