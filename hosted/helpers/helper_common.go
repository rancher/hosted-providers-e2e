package helpers

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/cloudcredentials"
	"github.com/rancher/shepherd/extensions/cloudcredentials/aws"
	"github.com/rancher/shepherd/extensions/cloudcredentials/azure"
	"github.com/rancher/shepherd/extensions/cloudcredentials/google"
	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/extensions/defaults"
	nodestat "github.com/rancher/shepherd/extensions/nodes"
	"github.com/rancher/shepherd/extensions/pipeline"
	"github.com/rancher/shepherd/extensions/users"
	password "github.com/rancher/shepherd/extensions/users/passwordgenerator"
	"github.com/rancher/shepherd/extensions/workloads/pods"
	"github.com/rancher/shepherd/pkg/config"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"github.com/rancher/shepherd/pkg/session"
	"github.com/rancher/shepherd/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func CommonSynchronizedBeforeSuite() {
	ginkgo.GinkgoLogr.Info("Using Common SynchronizedBeforeSuite ...")

	rancherConfig := new(rancher.Config)

	// Attempt at manually loading and updating the rancher config to avoid `nil map entry assignment`
	config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)

	rancherConfig.Host = RancherHostname
	time.Sleep(2 * time.Second)

	token, err := pipeline.CreateAdminToken(RancherPassword, rancherConfig)
	Expect(err).To(BeNil())
	rancherConfig.AdminToken = token

	config.UpdateConfig(rancher.ConfigurationFileKey, rancherConfig)

	switch Provider {
	case "aks":
		credentialConfig := new(cloudcredentials.AzureCredentialConfig)
		config.LoadAndUpdateConfig("azureCredentials", credentialConfig, func() {
			credentialConfig.ClientID = os.Getenv("AKS_CLIENT_ID")
			credentialConfig.SubscriptionID = os.Getenv("AKS_SUBSCRIPTION_ID")
			credentialConfig.ClientSecret = os.Getenv("AKS_CLIENT_SECRET")
		})
	case "eks":
		credentialConfig := new(cloudcredentials.AmazonEC2CredentialConfig)
		config.LoadAndUpdateConfig("awsCredentials", credentialConfig, func() {
			credentialConfig.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
			credentialConfig.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
			credentialConfig.DefaultRegion = GetEKSRegion()
		})

	case "gke":
		credentialConfig := new(cloudcredentials.GoogleCredentialConfig)
		config.LoadAndUpdateConfig("googleCredentials", credentialConfig, func() {
			credentialConfig.AuthEncodedJSON = os.Getenv("GCP_CREDENTIALS")
		})
	}

}

func CommonBeforeSuite() Context {
	ginkgo.GinkgoLogr.Info("Using Common BeforeSuite ...")

	rancherConfig := new(rancher.Config)
	config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)

	testSession := session.NewSession()
	rancherAdminClient, err := rancher.NewClient(rancherConfig.AdminToken, testSession)
	Expect(err).To(BeNil())

	setting := new(management.Setting)
	resp, err := rancherAdminClient.Management.Setting.ByID("server-url")
	Expect(err).To(BeNil())

	setting.Source = "env"
	setting.Value = fmt.Sprintf("https://%s", RancherHostname)
	_, err = rancherAdminClient.Management.Setting.Update(resp, setting)
	Expect(err).To(BeNil())

	var cloudCredential *cloudcredentials.CloudCredential

	switch Provider {
	case "aks":
		cloudCredential, err = azure.CreateAzureCloudCredentials(rancherAdminClient)
		Expect(err).To(BeNil())
	case "eks":
		cloudCredential, err = aws.CreateAWSCloudCredentials(rancherAdminClient)
		Expect(err).To(BeNil())
	case "gke":
		cloudCredential, err = google.CreateGoogleCloudCredentials(rancherAdminClient)
		Expect(err).To(BeNil())
	}

	return Context{
		CloudCred:          cloudCredential,
		RancherAdminClient: rancherAdminClient,
		Session:            testSession,
		ClusterCleanup:     clusterCleanup,
	}
}

func CreateStdUserClient(ctx *Context) {
	ginkgo.GinkgoLogr.Info("Creating Std User client ...")

	var stduser = namegen.AppendRandomString("stduser-")
	var stduserpassword = password.GenerateUserPassword("testpass-")
	newuser := &management.User{
		Username: stduser,
		Password: stduserpassword,
		Name:     stduser,
		Enabled:  pointer.Bool(true),
	}

	stdUser, err := users.CreateUserWithRole(ctx.RancherAdminClient, newuser, "user")
	Expect(err).To(BeNil())

	stdUser.Password = newuser.Password
	stdUserClient, err := ctx.RancherAdminClient.AsUser(stdUser)
	Expect(err).To(BeNil())

	var cloudCredential *cloudcredentials.CloudCredential
	switch Provider {
	case "aks":
		cloudCredential, err = azure.CreateAzureCloudCredentials(stdUserClient)
		Expect(err).To(BeNil())
	case "eks":
		cloudCredential, err = aws.CreateAWSCloudCredentials(stdUserClient)
		Expect(err).To(BeNil())
	case "gke":
		cloudCredential, err = google.CreateGoogleCloudCredentials(stdUserClient)
		Expect(err).To(BeNil())
	}

	ctx.CloudCred = cloudCredential
	ctx.StdUserClient = stdUserClient
}

// WaitUntilClusterIsReady waits until the cluster is in a Ready state,
// fetch the cluster again once it's ready so that it has everything up to date and then return it.
// For e.g. once the cluster has been updated, it contains information such as Version.GitVersion which it does not have before it's ready
// If the cluster is imported; it also updates the ProviderConfig with ProviderStatus.UpstreamSpec data
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
	var updatedCluster *management.Cluster
	updatedCluster, err = client.Management.Cluster.ByID(cluster.ID)
	if err != nil {
		// returning the value as returned via ByID().
		return updatedCluster, err
	}

	// Workaround to null values in ProviderConfig for an imported cluster
	// Ref: https://github.com/rancher/aks-operator/issues/251 (won't fix)
	if IsImport {
		switch Provider {
		case "aks":
			updatedCluster.AKSConfig = updatedCluster.AKSStatus.UpstreamSpec
		case "gke":
			updatedCluster.GKEConfig = updatedCluster.GKEStatus.UpstreamSpec
		case "eks":
			updatedCluster.EKSConfig = updatedCluster.EKSStatus.UpstreamSpec
		}
	}
	return updatedCluster, nil

}

// ClusterIsReadyChecks runs the basic checks on a cluster such as cluster name, service account, nodes and pods check
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

	metadataLabels := map[string]string{
		"owner":          "hosted-providers-qa-ci-" + testuser.Username,
		"testfilenumber": filename,
	}

	if !clusterCleanup {
		metadataLabels["janitor-ignore"] = "true"
	} else {
		metadataLabels["aws-janitor/marked-for-deletion"] = "true"
	}
	return metadataLabels
}

func SetTempKubeConfig(clusterName string) {
	downstreamKubeconfig := os.Getenv(DownstreamKubeconfig(clusterName))
	if downstreamKubeconfig == "" {
		tmpKubeConfig, err := os.CreateTemp("", clusterName)
		Expect(err).To(BeNil())
		downstreamKubeconfig = tmpKubeConfig.Name()
		_ = os.Setenv(DownstreamKubeconfig(clusterName), downstreamKubeconfig)
	}
	_ = os.Setenv("KUBECONFIG", downstreamKubeconfig)
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

// CheckMapElements checks if map1 keys are subset of map2
func CheckMapKeys(map1, map2 map[string]string) (exists bool) {
	for key := range map1 {
		_, exists = map2[key]
	}
	return
}
