package support_matrix_test

import (
	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
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
