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

package support_matrix_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"fmt"

	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SupportMatrixImport", func() {

	for _, version := range availableVersionList {
		version := version

		When(fmt.Sprintf("a cluster is created with kubernetes version %s", version), func() {
			var (
				clusterName string
				cluster     *management.Cluster
			)
			BeforeEach(func() {
				clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
				var err error
				err = helper.CreateEKSClusterOnAWS(region, clusterName, version, "1", helpers.GetCommonMetadataLabels())
				Expect(err).To(BeNil())
				cluster, err = helper.ImportEKSHostedCluster(ctx.StdUserClient, clusterName, ctx.CloudCredID, region)
				Expect(err).To(BeNil())
				// Requires RancherAdminClient
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})
			AfterEach(func() {
				if ctx.ClusterCleanup && cluster != nil {
					err := helper.DeleteEKSHostCluster(cluster, ctx.StdUserClient)
					Expect(err).To(BeNil())
					err = helper.DeleteEKSClusterOnAWS(region, clusterName)
					Expect(err).To(BeNil())
				} else {
					fmt.Println("Skipping downstream cluster deletion: ", clusterName)
				}
			})

			It("should successfully import the cluster", func() {
				// Report to Qase
				testCaseID = 70

				helpers.ClusterIsReadyChecks(cluster, ctx.StdUserClient, clusterName)
			})
		})
	}
})
