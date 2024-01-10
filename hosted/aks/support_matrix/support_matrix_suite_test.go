package support_matrix_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	availableVersionList []string
	ctx                  helpers.Context
	location             = helpers.GetAKSLocation()
)

func TestSupportMatrix(t *testing.T) {
	RegisterFailHandler(Fail)
	var err error
	// TODO: Fix this behavior; move to BeforeSuite.
	ctx, err = helpers.CommonBeforeSuite("aks")
	Expect(err).To(BeNil())
	availableVersionList, err = helper.ListSingleVariantAKSAvailableVersions(ctx.RancherClient, ctx.CloudCred.ID, location)
	Expect(err).To(BeNil())
	RunSpecs(t, "SupportMatrix Suite")
}
