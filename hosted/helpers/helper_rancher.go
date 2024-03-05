package helpers

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	rancherhelper "github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
)

// GetRancherVersion returns the rancher version information as RancherVersionInfo
// Sample Response from /rancherversion API endpoint:
// 2.8-head {"Version":"6aae4eeee","GitCommit":"6aae4eeee","RancherPrime":"false"}
// 2.8.2 {"Version":"v2.8.2","GitCommit":"2f7113dc3","RancherPrime":"false"}
func GetRancherVersion() (versionInfo RancherVersionInfo) {
	url := fmt.Sprintf("%s://%s/rancherversion", "https", RancherHostname)
	httpClient := http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	response, err := httpClient.Get(url)
	Expect(err).To(BeNil())
	defer func() {
		_ = response.Body.Close()
	}()

	var bodyBytes []byte
	bodyBytes, err = io.ReadAll(response.Body)
	Expect(err).To(BeNil())
	err = json.Unmarshal(bodyBytes, &versionInfo)
	Expect(err).To(BeNil())

	if versionInfo.Version == versionInfo.GitCommit {
		versionInfo.Devel = true
	}
	return
}

// DeployRancherManager deploys Rancher. If checkPods is true, it waits until all the necessary pods are running
// fullVersion: devel/2.8, 2.8.2, 2.8.1-rc3
func DeployRancherManager(fullVersion string, checkPods bool) {
	channelName := "rancher-" + RancherChannel

	var version, headVersion string
	versionSplit := strings.Split(fullVersion, "/")
	version = versionSplit[0]
	if len(versionSplit) == 2 {
		headVersion = versionSplit[1]
	}

	// Set flags for Rancher Manager installation
	flags := []string{
		"upgrade", "--install", "rancher", channelName + "/rancher",
		"--namespace", CattleSystemNS,
		"--create-namespace",
		"--set", "hostname=" + RancherHostname,
		"--set", "bootstrapPassword=" + RancherPassword,
		"--set", "replicas=1",
		"--set", "global.cattle.psp.enabled=false",
		"--wait",
	}

	// Set specified version if needed
	if version != "" && version != "latest" {
		if version == "devel" {
			flags = append(flags,
				"--devel",
				"--set", "rancherImageTag=v"+headVersion+"-head",
			)
		} else if strings.Contains(version, "-rc") {
			flags = append(flags,
				"--devel",
				"--version", version,
			)
		} else {
			flags = append(flags, "--version", version)
		}
	}

	ginkgo.GinkgoLogr.Info(fmt.Sprintf("Deploying rancher: %v", flags))
	err := kubectl.RunHelmBinaryWithCustomErr(flags...)
	Expect(err).To(BeNil())

	if checkPods {
		// Wait for all pods to be started
		checkList := [][]string{
			{CattleSystemNS, "app=rancher"},
			{"cattle-fleet-local-system", "app=fleet-agent"},
			{CattleSystemNS, "app=rancher-webhook"},
		}
		Eventually(func() error {
			return rancherhelper.CheckPod(kubectl.New(), checkList)
		}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())

		// A bit dirty be better to wait a little here for all to be correctly started
		time.Sleep(2 * time.Minute)
	}
}
