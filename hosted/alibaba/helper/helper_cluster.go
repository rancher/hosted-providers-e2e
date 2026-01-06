package helper

import (
	"fmt"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/tools"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	ali "github.com/rancher/shepherd/extensions/clusters/alibaba"

	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"

	cs "github.com/alibabacloud-go/cs-20151215/v5/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/rancher/shepherd/pkg/config"
	"k8s.io/utils/pointer"
)

// CreateACKHostedCluster creates an ACK hosted cluster
func CreateAlibabaHostedCluster(client *rancher.Client, displayName, cloudCredentialID, kubernetesVersion, region string, updateFunc func(clusterConfig *ali.ClusterConfig)) (*management.Cluster, error) {
	// Load aliClusterConfig from YAML config
	var aliClusterConfig ali.ClusterConfig
	config.LoadConfig(ali.ALIClusterConfigConfigurationFileKey, &aliClusterConfig)
	if aliClusterConfig.ClusterName == "" {
		return nil, fmt.Errorf("aliClusterConfig not loaded or missing required fields")
	}

	// Override with function args if provided (only if non-empty)
	if region != "" {
		aliClusterConfig.RegionID = region
	}
	aliClusterConfig.ClusterName = displayName
	aliClusterConfig.AlibabaCredentialSecret = cloudCredentialID
	if kubernetesVersion != "" {
		aliClusterConfig.KubernetesVersion = kubernetesVersion
	}
	aliClusterConfig.Imported = false
	aliClusterConfig.ClusterSpec = "ack.pro.small"

	if updateFunc != nil {
		updateFunc(&aliClusterConfig)
	}

	// // Debug logging for region
	// ginkgo.GinkgoLogr.Info(fmt.Sprintf("Alibaba provisioning: region argument='%s', aliClusterConfig.RegionID='%s'", region, aliClusterConfig.RegionID))

	// // Debug logging for node pools
	// for i, np := range aliClusterConfig.NodePools {
	// 	ginkgo.GinkgoLogr.Info(fmt.Sprintf("NodePool[%d]: Name='%s', ImageId='%s', ImageType='%s', InstanceTypes=%v", i, np.Name, np.ImageId, np.ImageType, np.InstanceTypes))
	// }

	// Map all fields from aliClusterConfig to AliClusterConfigSpec
	aliSpec := &management.AliClusterConfigSpec{
		ClusterName:             aliClusterConfig.ClusterName,
		ClusterType:             aliClusterConfig.ClusterType,
		ClusterSpec:             aliClusterConfig.ClusterSpec,
		KubernetesVersion:       aliClusterConfig.KubernetesVersion,
		EndpointPublicAccess:    aliClusterConfig.EndpointPublicAccess,
		Imported:                aliClusterConfig.Imported,
		RegionID:                aliClusterConfig.RegionID,
		ZoneIDs:                 aliClusterConfig.ZoneIDs,
		AlibabaCredentialSecret: aliClusterConfig.AlibabaCredentialSecret,
		Addons:                  mapAliAddons(aliClusterConfig.Addons),
		SNATEntry:               aliClusterConfig.SNATEntry,
		ServiceCIDR:             aliClusterConfig.ServiceCIDR,
		ResourceGroupID:         aliClusterConfig.ResourceGroupID,
		ProxyMode:               aliClusterConfig.ProxyMode,
		NodePools:               ali.MapAliNodePoolsFromAliNodePool(aliClusterConfig.NodePools),
		// Add more fields as needed (podVswitchIds, vswitchIds, vpcId, securityGroupId, etc.)
	}

	cluster := &management.Cluster{
		DockerRootDir: "/var/lib/docker",
		AliConfig:     aliSpec,
		Name:          displayName,
		// Add more top-level fields if needed (labels, annotations, etc.)
	}

	clusterResp, err := client.Management.Cluster.Create(cluster)
	if err != nil {
		return nil, err
	}
	return clusterResp, nil
}

func CreateACKClusterOnAlibaba(csClient *cs.Client, region string, clusterName string, k8sVersion string, nodes string, resourceGroupId string, tags map[string]string, extraArgs ...string) (string, error) {

	// Load aliClusterConfig from YAML config
	var aliClusterConfig ali.ClusterConfig
	config.LoadConfig(ali.ALIClusterConfigConfigurationFileKey, &aliClusterConfig)

	if k8sVersion != "" {
		aliClusterConfig.KubernetesVersion = k8sVersion
	}

	nodepool := &cs.Nodepool{
		KubernetesConfig: &cs.NodepoolKubernetesConfig{
			Runtime:        tea.String(aliClusterConfig.NodePools[0].Runtime),
			RuntimeVersion: tea.String(aliClusterConfig.NodePools[0].RuntimeVersion),
		},
		NodepoolInfo: &cs.NodepoolNodepoolInfo{
			Name:            tea.String(aliClusterConfig.NodePools[0].Name),
			ResourceGroupId: tea.String(resourceGroupId),
		},
		ScalingGroup: &cs.NodepoolScalingGroup{
			VswitchIds:         tea.StringSlice(aliClusterConfig.VSwitchIDs),
			DesiredSize:        tea.Int64(aliClusterConfig.NodePools[0].DesiredSize),
			InstanceTypes:      tea.StringSlice([]string{aliClusterConfig.NodePools[0].InstanceTypes[0]}),
			SystemDiskCategory: tea.String(aliClusterConfig.NodePools[0].SystemDiskCategory),
			SystemDiskSize:     tea.Int64(aliClusterConfig.NodePools[0].SystemDiskSize),
			DataDisks: []*cs.DataDisk{
				{
					Category:  tea.String(aliClusterConfig.NodePools[0].DataDisks[0].Category),
					Size:      tea.Int64(int64(aliClusterConfig.NodePools[0].DataDisks[0].Size)),
					Encrypted: tea.String("false"),
				},
			},
		},
	}
	req := &cs.CreateClusterRequest{
		Name:                 tea.String(clusterName),
		ClusterType:          tea.String(aliClusterConfig.ClusterType),
		ClusterSpec:          tea.String(aliClusterConfig.ClusterSpec),
		KubernetesVersion:    tea.String(k8sVersion),
		ServiceCidr:          tea.String(aliClusterConfig.ServiceCIDR),
		Vpcid:                tea.String(aliClusterConfig.VpcID),
		VswitchIds:           tea.StringSlice(aliClusterConfig.VSwitchIDs),
		PodVswitchIds:        tea.StringSlice(aliClusterConfig.PodVswitchIDs),
		Nodepools:            []*cs.Nodepool{nodepool},
		SnatEntry:            tea.Bool(true),
		RegionId:             tea.String(region),
		ResourceGroupId:      tea.String(resourceGroupId),
		EndpointPublicAccess: tea.Bool(true),
		Addons: []*cs.Addon{
			{
				Name: tea.String(aliClusterConfig.Addons[0].Name),
			},
		},
	}

	// Optional: set additional parameters using setters
	req.SetTimeoutMins(20)

	resp, err := csClient.CreateCluster(req)
	if err != nil {
		return "", err
	}

	fmt.Printf("Cluster creation started. ClusterID: %s, TaskID: %s\n",
		tea.StringValue(resp.Body.ClusterId),
		tea.StringValue(resp.Body.TaskId),
	)

	err = waitForClusterReady(csClient, tea.StringValue(resp.Body.ClusterId), 15)
	if err != nil {
		return "", err
	}
	return tea.StringValue(resp.Body.ClusterId), err
}

func waitForClusterReady(csClient *cs.Client, clusterID string, timeoutMinutes int) error {
	timeout := time.After(time.Duration(timeoutMinutes) * time.Minute)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout reached waiting for cluster %s to be ready", clusterID)

		case <-ticker.C:
			req := &cs.DescribeClustersV1Request{
				ClusterId: tea.String(clusterID),
			}

			resp, err := csClient.DescribeClustersV1(req)
			if err != nil {
				log.Printf("Error describing cluster: %v", err)
				continue
			}

			if resp.Body == nil || len(resp.Body.Clusters) == 0 {
				log.Printf("No cluster info found for %s", clusterID)
				continue
			}

			clusterStatus := tea.StringValue(resp.Body.Clusters[0].State)
			fmt.Printf("Cluster %s status: %s\n", clusterID, clusterStatus)

			switch clusterStatus {
			case "running":
				fmt.Println("Waiting for nodepools...")
			case "failed":
				return fmt.Errorf("cluster creation failed")
			case "deleted":
				return fmt.Errorf("cluster is deleted")
			default:
				continue // wait for cluster to be in running state
			}

			// Once cluster is running, check nodepools
			nodePoolsResp, err := csClient.DescribeClusterNodePools(tea.String(clusterID), &cs.DescribeClusterNodePoolsRequest{})
			if err != nil {
				log.Printf("Error describing node pools: %v", err)
				continue
			}

			if nodePoolsResp.Body == nil || len(nodePoolsResp.Body.Nodepools) == 0 {
				continue
			}

			allActive := true
			for _, np := range nodePoolsResp.Body.Nodepools {
				if np == nil {
					continue
				}

				name := tea.StringValue(np.NodepoolInfo.Name)
				status := tea.StringValue(np.Status.State)

				fmt.Printf("Nodepool %s status: %s\n", name, status)

				switch status {
				case "active":
					fmt.Printf("Nodepool %s is active\n", name)
				case "scaling", "removing", "deleting", "updating":
					allActive = false
				default:
					allActive = false
				}
			}
			if allActive {
				fmt.Println("Cluster and all nodepools are active!")
				return nil
			}
		}
	}
}

func waitForClusterDeletion(csClient *cs.Client, clusterID string, timeoutMinutes int) error {
	timeout := time.After(time.Duration(timeoutMinutes) * time.Minute)
	ticker := time.NewTicker(15 * time.Second) // check every 15 seconds
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout reached waiting for cluster %s to be deleted", clusterID)
		case <-ticker.C:
			req := &cs.DescribeClustersV1Request{
				ClusterId: tea.String(clusterID),
			}

			resp, err := csClient.DescribeClustersV1(req)
			if err != nil {
				log.Printf("Error describing cluster: %v", err)
				continue
			}

			// Check if the Clusters slice is empty. If it is, the cluster is likely deleted.
			if resp.Body == nil || resp.Body.Clusters == nil || len(resp.Body.Clusters) == 0 {
				fmt.Printf("Cluster %s no longer found, it's deleted.\n", clusterID)
				return nil
			}

			status := tea.StringValue(resp.Body.Clusters[0].State)
			fmt.Printf("Cluster %s status: %s\n", clusterID, status)

			switch status {
			case "deleted":
				fmt.Println("cluster deleted")
				return nil
			case "delete_failed":
				return fmt.Errorf("cluster deletion failed")
			}
		}
	}
}

func ImportACKHostedCluster(client *rancher.Client, clusterName, cloudCredentialID, region string, clusterId string) (*management.Cluster, error) {
	cluster := &management.Cluster{
		DockerRootDir: "/var/lib/docker",

		AliConfig: &management.AliClusterConfigSpec{
			ClusterID:               clusterId,
			AlibabaCredentialSecret: cloudCredentialID,
			ClusterName:             clusterName,
			Imported:                true,
			RegionID:                region,
		},
		Name: clusterName,
	}

	clusterResp, err := client.Management.Cluster.Create(cluster)
	if err != nil {
		return nil, err
	}
	return clusterResp, nil
}

// Helper to map []Addon to []management.AliAddon
func mapAliAddons(addons []ali.Addon) []management.AliAddon {
	out := make([]management.AliAddon, len(addons))
	for i, a := range addons {
		out[i] = management.AliAddon{Name: a.Name}
	}
	return out
}

func DeleteACKClusteronAlibaba(csClient *cs.Client, clusterId string) error {
	_, err := csClient.DeleteClusterWithOptions(
		tea.String(clusterId),
		&cs.DeleteClusterRequest{},
		map[string]*string{},
		&util.RuntimeOptions{},
	)
	if err != nil {
		return (fmt.Errorf("failed to delete cluster: %w", err))
	}
	fmt.Println("Cluster deletion initiated successfully.")

	err = waitForClusterDeletion(csClient, clusterId, 15)
	return err
}

// newClusterUpdatePayload creates a new management.Cluster object prepared for an update operation.
// It initializes the payload with the cluster's name and a deep copy of its AliConfig.
func newClusterUpdatePayload(cluster *management.Cluster) *management.Cluster {
	payload := new(management.Cluster)
	payload.Name = cluster.Name
	aliConfigCopy := *cluster.AliConfig
	payload.AliConfig = &aliConfigCopy
	return payload
}

// populateNodePoolImageIDs ensures that all node pools in the given cluster's AliConfig
// have their ImageID populated from the default configuration template.
func populateNodePoolImageIDs(cluster *management.Cluster) {
	var aliConfigTemplate ali.ClusterConfig
	config.LoadConfig(ali.ALIClusterConfigConfigurationFileKey, &aliConfigTemplate)
	templateImageID := aliConfigTemplate.NodePools[0].ImageId

	for i := range cluster.AliConfig.NodePools {
		cluster.AliConfig.NodePools[i].ImageID = templateImageID
	}
}

// DeleteALIHostCluster deletes the ALI cluster
func DeleteACKHostCluster(cluster *management.Cluster, client *rancher.Client) error {
	return client.Management.Cluster.Delete(cluster)
}

// AddNodePool adds a node pool to the ALI cluster
// increaseBy: number of node pools to add
// imageType: optional image type for new node pool
// wait, checkClusterConfig: control wait and validation
func AddNodePool(cluster *management.Cluster, client *rancher.Client, increaseBy int, imageType string, wait, checkClusterConfig bool) (*management.Cluster, error) {
	currentNodePoolNumber := len(cluster.AliConfig.NodePools)

	// Use the first node pool as a template
	var aliConfigTemplate ali.ClusterConfig
	config.LoadConfig(ali.ALIClusterConfigConfigurationFileKey, &aliConfigTemplate)

	npTemplate := aliConfigTemplate.NodePools[0]
	if imageType != "" {
		npTemplate.ImageType = imageType
	}

	// Create a deep copy of AliConfig first to avoid modifying the original cluster
	upgradedCluster := newClusterUpdatePayload(cluster)

	// Ensure ImageID is populated for all existing node pools in the update payload
	populateNodePoolImageIDs(upgradedCluster)

	// Add new node pools to the copy
	for i := 1; i <= increaseBy; i++ {
		newNodepool := management.AliNodePool{
			Name:               fmt.Sprintf("np-%d", currentNodePoolNumber+i),
			InstanceTypes:      npTemplate.InstanceTypes,
			DesiredSize:        pointer.Int64(npTemplate.DesiredSize),
			SystemDiskCategory: npTemplate.SystemDiskCategory,
			SystemDiskSize:     npTemplate.SystemDiskSize,
			ImageID:            npTemplate.ImageId,
			ImageType:          npTemplate.ImageType,
			Runtime:            npTemplate.Runtime,
			RuntimeVersion:     npTemplate.RuntimeVersion,
			DataDisks:          ali.MapAliDataDisks(npTemplate.DataDisks),
		}
		upgradedCluster.AliConfig.NodePools = append(upgradedCluster.AliConfig.NodePools, newNodepool)
	}

	cluster, err := client.Management.Cluster.Update(cluster, upgradedCluster)
	if err != nil {
		return nil, err
	}

	if checkClusterConfig {
		Expect(len(cluster.AliConfig.NodePools)).Should(BeNumerically("==", currentNodePoolNumber+increaseBy))
		for i := range cluster.AliConfig.NodePools {
			Expect(cluster.AliConfig.NodePools[i].Name).To(Equal(upgradedCluster.AliConfig.NodePools[i].Name))
		}
	}

	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}

	if checkClusterConfig {
		// Check if the desired config has been applied in Rancher
		Eventually(func() int {
			ginkgo.GinkgoLogr.Info("Waiting for the total node pool count to increase in AliStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.AliStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(helpers.Timeout), 10*time.Second).Should(BeNumerically("==", currentNodePoolNumber+increaseBy))

		for i := range cluster.AliStatus.UpstreamSpec.NodePools {
			Expect(cluster.AliStatus.UpstreamSpec.NodePools[i].Name).To(Equal(upgradedCluster.AliConfig.NodePools[i].Name))
		}
	}
	return cluster, nil
}

// ScaleNodePool modifies the DesiredSize of all node pools in the ALI cluster as defined by nodeCount
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
func ScaleNodePool(cluster *management.Cluster, client *rancher.Client, nodeCount int64, wait, checkClusterConfig bool) (*management.Cluster, error) {
	updatePayload := new(management.Cluster)
	updatePayload = newClusterUpdatePayload(cluster)

	// Ensure ImageID is populated for all node pools in the update payload
	populateNodePoolImageIDs(updatePayload)

	for i := range updatePayload.AliConfig.NodePools {
		updatePayload.AliConfig.NodePools[i].DesiredSize = pointer.Int64(nodeCount)
		updatePayload.AliConfig.NodePools[i].MaxInstances = pointer.Int64(nodeCount)
	}

	cluster, err := client.Management.Cluster.Update(cluster, updatePayload)
	if err != nil {
		return nil, err
	}

	if checkClusterConfig {
		// Check if the desired config is set correctly
		configNodePools := cluster.AliConfig.NodePools
		for i := range configNodePools {
			if configNodePools[i].DesiredSize != nil {
				Expect(*configNodePools[i].DesiredSize).To(BeNumerically("==", nodeCount))
			} else {
				ginkgo.GinkgoLogr.Info(fmt.Sprintf("DesiredSize is nil for node pool %d", i))
			}
		}
	}

	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}

	if checkClusterConfig {
		// check that the desired config is applied on Rancher
		Eventually(func() bool {
			ginkgo.GinkgoLogr.Info("Waiting for the node count change to appear in AliStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			upstreamNodePools := cluster.AliStatus.UpstreamSpec.NodePools
			for i := range upstreamNodePools {
				if np := upstreamNodePools[i]; *np.DesiredSize != nodeCount {
					return false
				}
			}
			return true
		}, tools.SetTimeout(helpers.Timeout), 10*time.Second).Should(BeTrue())
	}

	return cluster, nil
}

// DeleteNodePool deletes a node pool from the ALI cluster
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
func DeleteNodePool(cluster *management.Cluster, client *rancher.Client, wait, checkClusterConfig bool) (*management.Cluster, error) {
	currentNodePoolNumber := len(cluster.AliConfig.NodePools)
	if len(cluster.AliConfig.NodePools) <= 1 {
		return cluster, fmt.Errorf("cannot delete node pool: only one node pool remains")
	}

	upgradedCluster := newClusterUpdatePayload(cluster)
	upgradedCluster.AliConfig.NodePools = upgradedCluster.AliConfig.NodePools[1:]

	// Ensure ImageID is populated for all remaining node pools in the update payload
	populateNodePoolImageIDs(upgradedCluster)

	cluster, err := client.Management.Cluster.Update(cluster, upgradedCluster)
	if err != nil {
		return nil, err
	}

	if checkClusterConfig {
		// Check if the desired config is set correctly
		Expect(len(cluster.AliConfig.NodePools)).Should(BeNumerically("==", currentNodePoolNumber-1))
		// The check for correct node pool names is complex after deletion, so we focus on the count.
	}
	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}
	if checkClusterConfig {
		// Check if the desired config has been applied in Rancher
		Eventually(func() int {
			ginkgo.GinkgoLogr.Info("Waiting for the total node pool count to decrease in AliStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.AliStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(helpers.Timeout), 10*time.Second).Should(BeNumerically("==", currentNodePoolNumber-1))
		// The check for correct node pool names is complex after deletion, so we focus on the count.
	}
	return cluster, nil
}

// UpgradeClusterKubernetesVersion upgrades the control plane k8s version to the value defined by upgradeToVersion.
// If checkClusterConfig is set to true, it will validate that the control plane has been upgraded successfully
func UpgradeClusterKubernetesVersion(cluster *management.Cluster, upgradeToVersion string, client *rancher.Client, upgradeNodePool, wait, checkClusterConfig bool) (*management.Cluster, error) {
	upgradedCluster := newClusterUpdatePayload(cluster)

	// Ensure ImageID is populated for all node pools in the update payload
	populateNodePoolImageIDs(upgradedCluster)

	var upgradeCP bool
	if cluster.AliConfig.KubernetesVersion != upgradeToVersion {
		upgradedCluster.AliConfig.KubernetesVersion = upgradeToVersion
		upgradeCP = true
	}

	ginkgo.GinkgoLogr.Info(fmt.Sprintf("Kubernetes version for cluster %s will be upgraded to %s", cluster.Name, upgradeToVersion))

	var err error
	cluster, err = client.Management.Cluster.Update(cluster, upgradedCluster)
	if err != nil {
		return nil, err
	}

	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}

	if checkClusterConfig {
		if upgradeCP {
			// Check if the desired config has been applied in Rancher
			Eventually(func() bool {
				ginkgo.GinkgoLogr.Info("Waiting for k8s upgrade to appear in AliStatus.UpstreamSpec & AliConfig ...")
				cluster, err = client.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.AliStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
			}, tools.SetTimeout(helpers.Timeout), 30*time.Second).Should(BeTrue())
		}
	}
	return cluster, nil
}

// ListALIAvailableVersions lists all available ALI Kubernetes versions
// This is a separate static list maintained by hosted-providers-e2e, similar to the UI lists.
func ListALIAllVersions(client *rancher.Client) (allVersions []string, err error) {
	if err != nil {
		// Log and continue with a safe default list; server-version is non-critical for ALI static list selection
		ginkgo.GinkgoLogr.Info(fmt.Sprintf("[WARN] failed to read rancher server-version: %v; falling back to default ALI version list", err))
	}

	allVersions = []string{"1.33.3-aliyun.1", "1.32.7-aliyun.1", "1.32.6-aliyun.1"}

	// as a safety net, we ensure all the versions are UI supported
	return helpers.FilterUIUnsupportedVersions(allVersions, client), nil
}

// GetK8sVersion returns the k8s version to be used by the test;
// this value can either be a variant of envvar DOWNSTREAM_K8S_MINOR_VERSION or the highest available version
// or second-highest minor version in case of upgrade scenarios
func GetK8sVersion(client *rancher.Client, forUpgrade bool) (string, error) {
	if k8sVersion := helpers.DownstreamK8sMinorVersion; k8sVersion != "" {
		return k8sVersion, nil
	}
	allVariants, err := ListALIAllVersions(client)
	if err != nil {
		return "", err
	}

	return helpers.DefaultK8sVersion(allVariants, forUpgrade)
}
