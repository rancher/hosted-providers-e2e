/*
Copyright © 2023 - 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package p1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

const (
	increaseBy = 1
)

var (
	ctx                     helpers.Context
	clusterName, k8sVersion string
	testCaseID              int64
	zone                    = helpers.GetGKEZone()
	project                 = helpers.GetGKEProjectID()
)

func TestP1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P1 Suite")
}

var _ = BeforeEach(func() {
	var err error
	ctx, err = helpers.CommonBeforeSuite(helpers.Provider)
	Expect(err).To(BeNil())
	//clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	clusterName = "auto-gke-hp-ci-ftyju"
	k8sVersion, err = helper.GetK8sVersion(ctx.RancherClient, project, ctx.CloudCred.ID, zone, "")
	Expect(err).To(BeNil())
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

// updateLoggingAndMonitoringServiceCheck tests updating `loggingService` and `monitoringService`
func updateLoggingAndMonitoringServiceCheck(ctx helpers.Context, cluster *management.Cluster, updateMonitoringValue, updateLoggingValue string) {
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig

	upgradedCluster.GKEConfig.LoggingService = &updateLoggingValue
	upgradedCluster.GKEConfig.MonitoringService = &updateMonitoringValue

	cluster, err := ctx.RancherClient.Management.Cluster.Update(cluster, &upgradedCluster)
	Expect(err).To(BeNil())
	err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
	Expect(err).To(BeNil())

	Expect(*cluster.GKEConfig.MonitoringService).To(BeEquivalentTo(updateMonitoringValue))
	Expect(*cluster.GKEConfig.LoggingService).To(BeEquivalentTo(updateLoggingValue))
}

// updateAutoScaling tests updating `autoscaling` for GKE node pools
func updateAutoScaling(ctx helpers.Context, cluster *management.Cluster, autoscale bool) {
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig
	for i := range upgradedCluster.GKEConfig.NodePools {
		upgradedCluster.GKEConfig.NodePools[i].Autoscaling = &management.GKENodePoolAutoscaling{
			Enabled: autoscale,
		}
	}

	cluster, err := ctx.RancherClient.Management.Cluster.Update(cluster, &upgradedCluster)
	Expect(err).To(BeNil())
	err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
	Expect(err).To(BeNil())

	for _, np := range upgradedCluster.GKEConfig.NodePools {
		Expect(np.Autoscaling.Enabled).To(BeEquivalentTo(autoscale))
	}
}
