package p0_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	namegen "github.com/rancher/rancher/tests/framework/pkg/namegenerator"
)

const (
	increaseBy = 1
)

var (
	ctx         helpers.Context
	clusterName string
	region      = helpers.GetEKSRegion()
	k8sVersion  = helpers.GetK8sVersion("eks")
)

func TestP0(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P0 Suite")
}

var _ = BeforeEach(func() {
	var err error
	ctx, err = helpers.CommonBeforeSuite("eks")
	Expect(err).To(BeNil())
	clusterName = namegen.AppendRandomString("ekshostcluster")
})
