package helpers

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	"github.com/rancher/shepherd/clients/rancher/catalog"
)

func AddRancherCharts() {
	err := kubectl.RunHelmBinaryWithCustomErr("repo", "add", catalog.RancherChartRepo, "https://charts.rancher.io")
	Expect(err).To(BeNil())
	err = kubectl.RunHelmBinaryWithCustomErr("repo", "add", fmt.Sprintf("rancher-%s", RancherChannel), fmt.Sprintf("https://releases.rancher.com/server-charts/%s", RancherChannel))
	Expect(err).To(BeNil())
}

func GetCurrentChartVersion() string {
	charts := ListProviderChart()
	if len(charts) == 0 {
		return ""
	}
	return charts[0].DerivedVersion
}

func GetDowngradeChartVersion(currentChartVersion string) string {
	var chartName string
	if charts := ListProviderChart(); len(charts) > 0 {
		chartName = charts[0].Name
	} else {
		ginkgo.GinkgoLogr.Info("Could not find downgrade chart; chart is not installed")
		return ""
	}
	chartVersions := ListChartVersions(chartName)
	for _, chartVersion := range chartVersions {
		if VersionCompare(chartVersion.DerivedVersion, currentChartVersion) == -1 {
			return chartVersion.DerivedVersion
		}
	}
	return ""
}

func DowngradeProviderChart(downgradeChartVersion string) {
	currentChartVersion := GetCurrentChartVersion()
	Expect(currentChartVersion).ToNot(BeEmpty())
	UpdateProviderChartsVersion(downgradeChartVersion)
	Expect(VersionCompare(downgradeChartVersion, currentChartVersion)).To(Equal(-1))
}

// WaitProviderChartInstallation waits until the current chart version compares to the input chartVersion
// compareTo value can be 0 if current == input; -1 if current < input; 1 if current > input; defaults to 0
// compareTo is helpful in case either of the chart version is unknown;
// for e.g. when after a rancher upgrade, if only the old chart version is known then we can wait until the current chart version is greater than it
func WaitProviderChartInstallation(chartVersion string, compareTo int) {
	if !(compareTo >= -1 && compareTo <= 1) {
		compareTo = 0
	}
	Eventually(func() int {
		currentChartVersion := GetCurrentChartVersion()
		if currentChartVersion == "" {
			return 10
		}
		return VersionCompare(currentChartVersion, chartVersion)
	}, tools.SetTimeout(1*time.Minute), 5*time.Second).Should(BeNumerically("==", compareTo))

}

func UpdateProviderChartsVersion(updateChartVersion string) {
	for _, chart := range ListProviderChart() {
		err := kubectl.RunHelmBinaryWithCustomErr("upgrade", "--install", chart.Name, fmt.Sprintf("%s/%s", catalog.RancherChartRepo, chart.Name), "--namespace", CattleSystemNS, "--version", updateChartVersion, "--wait")
		if err != nil {
			Expect(err).To(BeNil(), "UpdateProviderChartsVersion Failed")
		}
	}
	WaitProviderChartInstallation(updateChartVersion, 0)
}

func UninstallProviderCharts() {
	for _, chart := range ListProviderChart() {
		args := []string{"uninstall", chart.Name, "--namespace", CattleSystemNS}
		err := kubectl.RunHelmBinaryWithCustomErr(args...)
		if err != nil {
			Expect(err).To(BeNil(), "Failed to uninstall chart %s", chart.Name)
		}
	}
}

// ListProviderChart lists the installed provider charts for a provider in cattle-system
func ListProviderChart() (operatorCharts []HelmChart) {
	cmd := exec.Command("helm", "list", "--namespace", CattleSystemNS, "-o", "json", "--filter", fmt.Sprintf("%s-operator", Provider))
	output, err := cmd.Output()
	Expect(err).To(BeNil(), "Failed to list chart %s", Provider)
	ginkgo.GinkgoLogr.Info(string(output))
	err = json.Unmarshal(output, &operatorCharts)
	Expect(err).To(BeNil(), "Failed to unmarshal chart %s", Provider)
	for i := range operatorCharts {
		operatorCharts[i].DerivedVersion = strings.TrimPrefix(operatorCharts[i].Chart, fmt.Sprintf("%s-", operatorCharts[i].Name))
	}
	return
}

// ListChartVersions lists all the available the chart version for a given chart name
func ListChartVersions(chartName string) (operatorCharts []HelmChart) {
	cmd := exec.Command("helm", "search", "repo", chartName, "--versions", "-ojson", "--devel")
	output, err := cmd.Output()
	Expect(err).To(BeNil())
	ginkgo.GinkgoLogr.Info(string(output))
	err = json.Unmarshal(output, &operatorCharts)
	Expect(err).To(BeNil())
	return
}

// VersionCompare compares Versions v to o:
// -1 == v is less than o
// 0 == v is equal to o
// 1 == v is greater than o
func VersionCompare(latestVersion, oldVersion string) int {
	latestVer, err := semver.ParseTolerant(latestVersion)
	Expect(err).To(BeNil())
	oldVer, err := semver.ParseTolerant(oldVersion)
	Expect(err).To(BeNil())
	return latestVer.Compare(oldVer)
}
