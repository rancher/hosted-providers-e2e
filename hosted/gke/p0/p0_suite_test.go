package p0_test

import (
	"context"
	v1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corev1 "k8s.io/api/core/v1"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	namegen "github.com/rancher/rancher/tests/framework/pkg/namegenerator"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	runtimeconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	cattleSystemNamespace = "cattle-system"
)

var (
	ctx          helpers.Context
	clusterName  string
	zone         = "us-central1-c"
	project      = os.Getenv("GKE_PROJECT_ID")
	k8sVersion   = "1.27.4-gke.900"
	increaseBy   = 1
	artifactsDir = "../_artifacts"
)

func TestP0(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P0 Suite")
}

var _ = BeforeEach(func() {
	var err error
	ctx, err = helpers.CommonBeforeSuite("gke")
	Expect(err).To(BeNil())
	clusterName = namegen.AppendRandomString("gkehostcluster")
})

var _ = AfterEach(func() {
	ctx := context.Background()
	By("Creating artifact directory")

	if _, err := os.Stat(artifactsDir); os.IsNotExist(err) {
		Expect(os.Mkdir(artifactsDir, os.ModePerm)).To(Succeed())
	}
	cfg, err := runtimeconfig.GetConfig()
	Expect(err).ToNot(HaveOccurred())

	cl, err := runtimeclient.New(cfg, runtimeclient.Options{})
	Expect(err).ToNot(HaveOccurred())

	By("Getting aks operator logs")

	podList := &corev1.PodList{}
	Expect(cl.List(ctx, podList, runtimeclient.MatchingLabels{
		"ke.cattle.io/operator": "gke",
	}, runtimeclient.InNamespace(cattleSystemNamespace),
	)).To(Succeed())

	for _, pod := range podList.Items {
		for _, container := range pod.Spec.Containers {
			output, err := kubectl.Run("logs", pod.Name, "-c", container.Name, "-n", pod.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(artifactsDir, pod.Name+"-"+container.Name+".log"), []byte(output), 0644)).To(Succeed())
		}
	}

	By("Getting GKE Clusters")

	gkeClusterList := &v1.GKEClusterConfigList{}
	Expect(cl.List(ctx, gkeClusterList, &runtimeclient.ListOptions{})).To(Succeed())

	for _, gkeCluster := range gkeClusterList.Items {
		output, err := yaml.Marshal(gkeCluster)
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(artifactsDir, "gke-cluster-config-"+gkeCluster.Name+".yaml"), output, 0644)).To(Succeed())
	}

	By("Getting Rancher Clusters")

	rancherClusterList := &managementv3.ClusterList{}
	Expect(cl.List(ctx, rancherClusterList, &runtimeclient.ListOptions{})).To(Succeed())

	for _, rancherCluster := range rancherClusterList.Items {
		output, err := yaml.Marshal(rancherCluster)
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(artifactsDir, "rancher-cluster-"+rancherCluster.Name+".yaml"), output, 0644)).To(Succeed())
	}
})
