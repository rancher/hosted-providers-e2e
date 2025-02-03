package helpers

import (
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	rancherhelper "github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
)

// DeployRancherManager deploys Rancher. If checkPods is true, it waits until all the necessary pods are running
// fullVersion: devel/2.9, 2.9.0, 2.9.0-rc1
func DeployRancherManager(rancherFullVersion string, checkPods bool) {
	rancherChannel, rancherVersion, rancherHeadVersion := GetRancherVersions(rancherFullVersion)
	// Set flags for Rancher Manager installation
	flags := []string{
		"upgrade", "--install", "rancher", "rancher-" + rancherChannel + "/rancher",
		"--namespace", CattleSystemNS,
		"--create-namespace",
		"--set", "hostname=" + RancherHostname,
		"--set", "bootstrapPassword=" + RancherPassword,
		"--set", "replicas=1",
		"--set", "global.cattle.psp.enabled=false",
		"--wait",
	}
	// Set specified version if needed
	if rancherVersion != "" && rancherVersion != "latest" {
		if rancherVersion == "devel" {
			flags = append(flags,
				"--devel",
				"--set", "rancherImageTag=v"+rancherHeadVersion+"-head",
			)
		} else if strings.Contains(rancherVersion, "-rc") {
			flags = append(flags,
				"--devel",
				"--version", rancherVersion,
			)
		} else {
			flags = append(flags, "--version", rancherVersion)
		}
	}
	ginkgo.GinkgoLogr.Info(fmt.Sprintf("Deploying rancher: %v", flags))
	err := kubectl.RunHelmBinaryWithCustomErr(flags...)
	Expect(err).To(BeNil())
	if checkPods {
		CheckRancherPods(true)
	}
}
func CheckRancherPods(wait bool) {
	// Wait for all pods to be started
	checkList := [][]string{
		{CattleSystemNS, "app=rancher"},
		{"cattle-fleet-system", "app=fleet-controller"},
		{CattleSystemNS, "app=rancher-webhook"},
	}
	Eventually(func() error {
		return rancherhelper.CheckPod(kubectl.New(), checkList)
	}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
	if wait {
		// A bit dirty be better to wait a little here for all to be correctly started
		time.Sleep(2 * time.Minute)
	}
}
