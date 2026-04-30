/*
Copyright © 2022 - 2026 SUSE LLC

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

package backup_restore_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	. "github.com/rancher-sandbox/qase-ginkgo"

	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	cs "github.com/alibabacloud-go/cs-20151215/v5/client"
	"github.com/rancher/hosted-providers-e2e/hosted/alibaba/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

const (
	increaseBy          = 1
	backupResourceName  = "hp-backup"
	restoreResourceName = "hp-restore"
)

var (
	testCaseID              int64
	clusterName, backupFile string
	clusterId               string
	ctx                     helpers.RancherContext
	cluster                 *management.Cluster
	csClient                *cs.Client
	region                  = helpers.GetACKRegion()
	k3sVersion              = os.Getenv("INSTALL_K3S_VERSION")
)

func TestBackupRestore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BackupRestore Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	helpers.CommonSynchronizedBeforeSuite()
	return nil
}, func() {
	ctx = helpers.CommonBeforeSuite()
	csClient = helpers.GetCSClient()
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

var _ = BeforeEach(func() {
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, false)
	Expect(err).To(BeNil())
	GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

	if helpers.IsImport {
		By("importing the cluster")
		clusterId, err = helper.CreateACKClusterOnAlibaba(csClient, region, clusterName, k8sVersion, "1", helpers.GetACKResourceGroupID(), helpers.GetCommonMetadataLabels())
		Expect(err).To(BeNil())
		cluster, err = helper.ImportACKHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region, clusterId)
		Expect(err).To(BeNil())
	} else {
		By("provisioning the cluster")
		cluster, err = helper.CreateAlibabaHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
		Expect(err).To(BeNil())
	}

	cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
	Expect(err).To(BeNil())
})

var _ = AfterEach(func() {
	if ctx.ClusterCleanup && cluster != nil {
		err := helper.DeleteACKHostCluster(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		if helpers.IsImport {
			err = helper.DeleteACKClusterOnAlibaba(csClient, clusterId)
			Expect(err).To(BeNil())
		}
	} else {
		fmt.Println("Skipping downstream cluster deletion: ", clusterName)
	}
})

func restoreNodesChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	helpers.ClusterIsReadyChecks(cluster, client, clusterName)

	var initialNodeCount int64
	if cluster.AliConfig != nil && len(cluster.AliConfig.NodePools) > 0 && cluster.AliConfig.NodePools[0].DesiredSize != nil {
		initialNodeCount = *cluster.AliConfig.NodePools[0].DesiredSize
	} else {
		Fail("initialNodeCount could not be determined from cluster's spec")
	}

	By("scaling up the NodePool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, client, initialNodeCount+increaseBy, true, true)
		Expect(err).To(BeNil())
	})

	By("adding a NodePool", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, client, increaseBy, "", true, true)
		Expect(err).To(BeNil())
	})
}

func BackupRestoreChecks(k *kubectl.Kubectl) {
	By("Checking hosted cluster is ready", func() {
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	By("Performing a backup", func() {
		backupFile = helpers.ExecuteBackup(k, backupResourceName)
	})

	By("Perform restore pre-requisites: Uninstalling k3s", func() {
		out, err := exec.Command("k3s-uninstall.sh").CombinedOutput()
		Expect(err).To(Not(HaveOccurred()), out)
	})

	By("Perform restore pre-requisites: Getting k3s ready", func() {
		helpers.InstallK3S(k, k3sVersion, "none", "none")
	})

	By("Performing a restore", func() {
		helpers.ExecuteRestore(k, restoreResourceName, backupFile)
	})

	By("Performing post migration installations: Installing CertManager", func() {
		helpers.InstallCertManager(k, "none", "none")
	})

	By("Performing post migration installations: Installing Rancher Manager", func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(helpers.RancherFullVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "none", "none")
	})

	By("Performing post migration installations: Checking Rancher Deployments", func() {
		helpers.CheckRancherDeployments(k)
	})

	By("Checking hosted cluster can be modified", func() {
		restoreNodesChecks(cluster, ctx.RancherAdminClient, clusterName)
	})
}
