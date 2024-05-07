package helpers

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/onsi/ginkgo/v2"
	nodestat "github.com/rancher/shepherd/extensions/nodes"
	"github.com/rancher/shepherd/extensions/pipeline"
	"github.com/rancher/shepherd/extensions/workloads/pods"

	. "github.com/onsi/gomega"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/cloudcredentials"
	"github.com/rancher/shepherd/extensions/cloudcredentials/aws"
	"github.com/rancher/shepherd/extensions/cloudcredentials/azure"
	"github.com/rancher/shepherd/extensions/cloudcredentials/google"
	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/extensions/defaults"
	"github.com/rancher/shepherd/pkg/config"
	"github.com/rancher/shepherd/pkg/session"
	"github.com/rancher/shepherd/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CommonBeforeSuite(cloud string) Context {

	rancherConfig := new(rancher.Config)
	// Workaround to ensure the config is
	config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)
	Expect(rancherConfig).ToNot(BeNil())

	config.LoadAndUpdateConfig(rancher.ConfigurationFileKey, rancherConfig, func() {
		rancherConfig.Host = RancherHostname
	})

	token, err := pipeline.CreateAdminToken(RancherPassword, rancherConfig)
	Expect(err).To(BeNil())

	config.LoadAndUpdateConfig(rancher.ConfigurationFileKey, rancherConfig, func() {
		rancherConfig.AdminToken = token
	})

	testSession := session.NewSession()
	rancherClient, err := rancher.NewClient(rancherConfig.AdminToken, testSession)
	Expect(err).To(BeNil())

	setting := new(management.Setting)
	resp, err := rancherClient.Management.Setting.ByID("server-url")
	Expect(err).To(BeNil())

	setting.Source = "env"
	setting.Value = fmt.Sprintf("https://%s", RancherHostname)
	resp, err = rancherClient.Management.Setting.Update(resp, setting)
	Expect(err).To(BeNil())

	var cloudCredential *cloudcredentials.CloudCredential
	switch cloud {
	case "aks":
		credentialConfig := new(cloudcredentials.AzureCredentialConfig)
		config.LoadAndUpdateConfig("azureCredentials", credentialConfig, func() {
			credentialConfig.ClientID = os.Getenv("AKS_CLIENT_ID")
			credentialConfig.SubscriptionID = os.Getenv("AKS_SUBSCRIPTION_ID")
			credentialConfig.ClientSecret = os.Getenv("AKS_CLIENT_SECRET")
		})
		cloudCredential, err = azure.CreateAzureCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
	case "eks":
		credentialConfig := new(cloudcredentials.AmazonEC2CredentialConfig)
		config.LoadAndUpdateConfig("awsCredentials", credentialConfig, func() {
			credentialConfig.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
			credentialConfig.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
			credentialConfig.DefaultRegion = GetEKSRegion()
		})
		cloudCredential, err = aws.CreateAWSCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
	case "gke":
		credentialConfig := new(cloudcredentials.GoogleCredentialConfig)
		config.LoadAndUpdateConfig("googleCredentials", credentialConfig, func() {
			credentialConfig.AuthEncodedJSON = os.Getenv("GCP_CREDENTIALS")
		})
		cloudCredential, err = google.CreateGoogleCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
	}

	return Context{
		CloudCred:      cloudCredential,
		RancherClient:  rancherClient,
		Session:        testSession,
		ClusterCleanup: clusterCleanup,
	}
}

// WaitUntilClusterIsReady waits until the cluster is in a Ready state,
// fetch the cluster again once it's ready so that it has everything up to date and then return it.
// For e.g. once the cluster has been updated, it contains information such as Version.GitVersion which it does not have before it's ready
func WaitUntilClusterIsReady(cluster *management.Cluster, client *rancher.Client) (*management.Cluster, error) {
	opts := metav1.ListOptions{FieldSelector: "metadata.name=" + cluster.ID, TimeoutSeconds: &defaults.WatchTimeoutSeconds}
	watchInterface, err := client.GetManagementWatchInterface(management.ClusterType, opts)
	if err != nil {
		return nil, err
	}

	watchFunc := clusters.IsHostedProvisioningClusterReady

	err = wait.WatchWait(watchInterface, watchFunc)
	if err != nil {
		return nil, err
	}
	return client.Management.Cluster.ByID(cluster.ID)
}

// ClusterIsReadyChecks runs the basic checks on a cluster such as cluster name, service account, nodes and pods check
// TODO(pvala): Use for other providers.
func ClusterIsReadyChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {

	ginkgo.By("checking cluster name is same", func() {
		Expect(cluster.Name).To(BeEquivalentTo(clusterName))
	})

	ginkgo.By("checking service account token secret", func() {
		success, err := clusters.CheckServiceAccountTokenSecret(client, clusterName)
		Expect(err).To(BeNil())
		Expect(success).To(BeTrue())
	})

	ginkgo.By("checking all management nodes are ready", func() {
		err := nodestat.AllManagementNodeReady(client, cluster.ID, Timeout)
		Expect(err).To(BeNil())
	})

	ginkgo.By("checking all pods are ready", func() {
		podErrors := pods.StatusPods(client, cluster.ID)
		Expect(podErrors).To(BeEmpty())
	})
}

// GetGKEZone fetches the value of GKE zone;
// it first obtains the value from env var GKE_ZONE, if the value is empty, it fetches the information from config file(cattle_config-import.yaml/cattle_config-provisioning.yaml)
// if none of the sources can provide a value, it returns the default value
func GetGKEZone() string {
	zone := os.Getenv("GKE_ZONE")
	if zone == "" {
		gkeConfig := new(management.GKEClusterConfigSpec)
		config.LoadConfig("gkeClusterConfig", gkeConfig)
		if gkeConfig.Zone != "" {
			zone = gkeConfig.Zone
		}
		if zone == "" {
			zone = "asia-south2-c"
		}
	}
	return zone
}

// GetAKSLocation fetches the value of AKS Region;
// it first obtains the value from env var AKS_REGION, if the value is empty, it fetches the information from config file(cattle_config-import.yaml/cattle_config-provisioning.yaml)
// if none of the sources can provide a value, it returns the default value
func GetAKSLocation() string {
	region := os.Getenv("AKS_REGION")
	if region == "" {
		aksClusterConfig := new(management.AKSClusterConfigSpec)
		config.LoadConfig("aksClusterConfig", aksClusterConfig)
		region = aksClusterConfig.ResourceLocation
		if region == "" {
			region = "centralindia"
		}
	}
	return region
}

// GetEKSRegion fetches the value of EKS Region;
// it first obtains the value from env var EKS_REGION, if the value is empty, it fetches the information from config file(cattle_config-import.yaml/cattle_config-provisioning.yaml)
// if none of the sources can provide a value, it returns the default value
func GetEKSRegion() string {
	region := os.Getenv("EKS_REGION")
	if region == "" {
		eksClusterConfig := new(management.EKSClusterConfigSpec)
		config.LoadConfig("eksClusterConfig", eksClusterConfig)
		region = eksClusterConfig.Region
		if region == "" {
			region = "ap-south-1"
		}
	}
	return region
}

// GetGKEProjectID returns the value of GKE project by fetching the value of env var GKE_PROJECT_ID
func GetGKEProjectID() string {
	return os.Getenv("GKE_PROJECT_ID")
}

// GetCommonMetadataLabels returns a list of common metadata labels/tabs
func GetCommonMetadataLabels() map[string]string {
	testuser, err := user.Current()
	Expect(err).To(BeNil())

	specReport := ginkgo.CurrentSpecReport()
	// filename indicates the filename and line number of the test
	// we only use this information instead of the ginkgo.CurrentSpecReport().FullText() because of the 63 character limit
	var filename string
	// Because of the way Support Matrix suites are designed, filename is not loaded at first, so we need to ensure it is non-empty before sanitizing it
	//E.g. line51_k8s_chart_support_provisioning_test
	if specReport.FileName() != "" {
		// Sanitize the filename to fit the label requirements for all the hosted providers
		fileSplit := strings.Split(specReport.FileName(), "/") // abstract the filename
		filename = fileSplit[len(fileSplit)-1]
		filename = strings.TrimSuffix(filename, ".go") // `.` is not allowed
		filename = strings.ToLower(filename)           // string must be in lowercase
		filename = fmt.Sprintf("line%d_%s", specReport.LineNumber(), filename)
	}

	return map[string]string{
		"owner":          "hosted-providers-qa-ci-" + testuser.Username,
		"testfilenumber": filename,
	}
}

func SetTempKubeConfig(clusterName string) {
	tmpKubeConfig, err := os.CreateTemp("", clusterName)
	Expect(err).To(BeNil())
	_ = os.Setenv(DownstreamKubeconfig(clusterName), tmpKubeConfig.Name())
	_ = os.Setenv("KUBECONFIG", tmpKubeConfig.Name())
}

// HighestK8sMinorVersionSupportedByUI returns the highest k8s version supported by UI
// TODO(pvala): Use this by default when fetching a list of k8s version for all the downstream providers.
func HighestK8sMinorVersionSupportedByUI(client *rancher.Client) (value string) {
	uiValue, err := client.Management.Setting.ByID("ui-k8s-default-version-range")
	Expect(err).To(BeNil())
	value = uiValue.Value
	Expect(value).ToNot(BeEmpty())
	value = strings.TrimPrefix(value, "<=v")
	value = strings.TrimSuffix(value, ".x")
	return value
}

// FilterUIUnsupportedVersions filters all k8s versions that are not supported by the UI
func FilterUIUnsupportedVersions(versions []string, client *rancher.Client) (filteredVersions []string) {
	maxValue := HighestK8sMinorVersionSupportedByUI(client)
	for _, version := range versions {
		// if the version is <= maxValue, then append it to the filtered list
		if comparison := VersionCompare(version, maxValue); comparison < 1 {
			filteredVersions = append(filteredVersions, version)
		} else if strings.Contains(version, maxValue) {
			filteredVersions = append(filteredVersions, version)
		}
	}
	return
}
