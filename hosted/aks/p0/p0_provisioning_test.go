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

package p0_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/pkg/config"

	"github.com/rancher/shepherd/extensions/clusters/aks"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P0Provisioning", func() {

	When("a cluster is created", func() {
		var cluster *management.Cluster

		BeforeEach(func() {
			var err error
			aksConfig := new(aks.ClusterConfig)
			config.LoadAndUpdateConfig(aks.AKSClusterConfigConfigurationFileKey, aksConfig, func() {
				aksConfig.ResourceGroup = clusterName
				dnsPrefix := clusterName + "-dns"
				aksConfig.DNSPrefix = &dnsPrefix
				aksConfig.ResourceLocation = location
				aksConfig.Tags = helper.GetTags()
				aksConfig.KubernetesVersion = &k8sVersion
			})
			cluster, err = aks.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			if ctx.ClusterCleanup {
				err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.DeleteAKSClusteronAzure(clusterName)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})
		It("should successfully provision the cluster & add, delete, scale nodepool", func() {
			// Report to Qase
			testCaseID = 173
			p0Checks(cluster)
		})

		It("should be able to upgrade k8s version of the cluster", func() {
			// Report to Qase
			testCaseID = 175
			upgradeK8sVersionCheck(cluster)
		})
	})

})
