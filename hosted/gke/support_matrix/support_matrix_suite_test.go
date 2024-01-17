package support_matrix_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	availableVersionList []string
	ctx                  helpers.Context
)

func TestSupportMatrix(t *testing.T) {
	RegisterFailHandler(Fail)
	var err error
	ctx, err = helpers.CommonBeforeSuite("gke")
	Expect(err).To(BeNil())
	availableVersionList, err = helper.ListSingleVariantGKEAvailableVersions(ctx.RancherClient, helpers.GetGKEProjectID(), ctx.CloudCred.ID, helpers.GetGKEZone(), "")
	Expect(err).To(BeNil())
	RunSpecs(t, "SupportMatrix Suite")
}
