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
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	nodestat "github.com/rancher/shepherd/extensions/nodes"
	"github.com/rancher/shepherd/extensions/workloads/pods"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	availableVersionList []string
	testCaseID           int64
	ctx                  helpers.Context
	location             = helpers.GetAKSLocation()
)

func TestSupportMatrix(t *testing.T) {
	RegisterFailHandler(Fail)
	helpers.CommonSynchronizedBeforeSuite()
	ctx = helpers.CommonBeforeSuite()
	helpers.CreateStdUserClient(&ctx)
	var err error
	availableVersionList, err = helper.ListSingleVariantAKSAvailableVersions(ctx.StdUserClient, ctx.CloudCred.ID, location)
	Expect(err).To(BeNil())
	RunSpecs(t, "SupportMatrix Suite")
}

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func supportMatrixChecks(client *rancher.Client, cluster *management.Cluster, clusterName string) {
	By("checking cluster name is same", func() {
		Expect(cluster.Name).To(BeEquivalentTo(clusterName))
	})

	By("checking service account token secret", func() {
		success, err := clusters.CheckServiceAccountTokenSecret(client, clusterName)
		Expect(err).To(BeNil())
		Expect(success).To(BeTrue())
	})

	By("checking all management nodes are ready", func() {
		err := nodestat.AllManagementNodeReady(client, cluster.ID, helpers.Timeout)
		Expect(err).To(BeNil())
	})

	By("checking all pods are ready", func() {
		podErrors := pods.StatusPods(client, cluster.ID)
		Expect(podErrors).To(BeEmpty())
	})
}
