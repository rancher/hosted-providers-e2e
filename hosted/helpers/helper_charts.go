package helpers

import (
	"encoding/json"
	"fmt"
	"os"
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

// AddRancherCharts adds the repo from which rancher operator charts can be installed
func AddRancherCharts() {
	err := kubectl.RunHelmBinaryWithCustomErr("repo", "add", catalog.RancherChartRepo, "https://charts.rancher.io")
	Expect(err).To(BeNil())
}

// GetCurrentOperatorChartVersion returns the current version of a Provider chart.
func GetCurrentOperatorChartVersion() string {
	charts := ListOperatorChart()
	if len(charts) == 0 {
		return ""
	}
	return charts[0].DerivedVersion
}

// GetDowngradeOperatorChartVersion returns a version to downgrade to from a given chart version.
func GetDowngradeOperatorChartVersion(currentChartVersion string) string {
	var chartName string
	if charts := ListOperatorChart(); len(charts) > 0 {
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
	currentChartVersion := GetCurrentOperatorChartVersion()
	Expect(currentChartVersion).ToNot(BeEmpty())
	UpdateOperatorChartsVersion(downgradeChartVersion)
	Expect(VersionCompare(downgradeChartVersion, currentChartVersion)).To(Equal(-1))
}

// WaitUntilOperatorChartInstallation waits until the current operator chart version compares to the input chartVersion using the comparator.
// comparator values can be >,<,<=,>=,==,!=
// compareTo value can be 0 if current == input; -1 if current < input; 1 if current > input; defaults to 0
// compareTo is helpful in case either of the chart version is unknown;
// for e.g. after a rancher upgrade, if only the old chart version is known then we can wait until the current chart version is greater than it
func WaitUntilOperatorChartInstallation(chartVersion, comparator string, compareTo int) {
	if comparator == "" {
		comparator = "=="
	}

	if !(compareTo >= -1 && compareTo <= 1) {
		compareTo = 0
	}
	Eventually(func() int {
		currentChartVersion := GetCurrentOperatorChartVersion()
		ginkgo.GinkgoLogr.Info(fmt.Sprintf("CurrentChartVersion: %s; comparing with %s to %s %d", currentChartVersion, chartVersion, comparator, compareTo))
		if currentChartVersion == "" {
			return 10
		}
		return VersionCompare(currentChartVersion, chartVersion)
	}, tools.SetTimeout(4*time.Minute), 3*time.Second).Should(BeNumerically(comparator, compareTo))

}

// UpdateOperatorChartsVersion updates the operator charts to a given chart version and validates that the current version is same as provided
func UpdateOperatorChartsVersion(updateChartVersion string) {
	for _, chart := range ListOperatorChart() {
		var chartSource string
		if Provider == "alibaba" {
			registry := getAlibabaOCIRegistry()
			Expect(registry).ToNot(BeEmpty(), "ALIBABA_OPERATOR_REGISTRY environment variable is not set")
			chartSource = fmt.Sprintf("%s/%s", registry, chart.Name)
		} else {
			chartSource = fmt.Sprintf("%s/%s", catalog.RancherChartRepo, chart.Name)
		}
		err := kubectl.RunHelmBinaryWithCustomErr("upgrade", "--install", chart.Name, chartSource, "--namespace", CattleSystemNS, "--version", updateChartVersion, "--wait")
		if err != nil {
			Expect(err).To(BeNil(), "UpdateOperatorChartsVersion Failed")
		}
	}
	Expect(VersionCompare(GetCurrentOperatorChartVersion(), updateChartVersion)).To(Equal(0))
}

// UninstallOperatorCharts uninstalls the operator charts
func UninstallOperatorCharts() {
	for _, chart := range ListOperatorChart() {
		args := []string{"uninstall", chart.Name, "--namespace", CattleSystemNS}
		err := kubectl.RunHelmBinaryWithCustomErr(args...)
		if err != nil {
			Expect(err).To(BeNil(), "Failed to uninstall chart %s", chart.Name)
		}
	}
}

// ListOperatorChart lists the installed provider charts for a provider in cattle-system; it fetches the provider value using Provider
func ListOperatorChart() (operatorCharts []HelmChart) {
	// The Alibaba chart is named rancher-ali-operator, not rancher-alibaba-operator
	providerFilter := Provider
	if Provider == "alibaba" {
		providerFilter = "ali"
	}
	cmd := exec.Command("helm", "list", "--namespace", CattleSystemNS, "-o", "json", "--filter", fmt.Sprintf("%s-operator", providerFilter))
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
func ListChartVersions(chartName string) (charts []HelmChart) {
	if Provider == "alibaba" {
		return listAlibabaChartVersions(chartName)
	}
	cmd := exec.Command("helm", "search", "repo", chartName, "--versions", "-ojson", "--devel")
	output, err := cmd.Output()
	Expect(err).To(BeNil())
	ginkgo.GinkgoLogr.Info(string(output))
	err = json.Unmarshal(output, &charts)
	Expect(err).To(BeNil())
	return
}

// listAlibabaChartVersions queries the OCI registry for available versions of an Alibaba chart.
// It probes multiple major version ranges (based on Rancher minor versions) to find available versions.
func listAlibabaChartVersions(chartName string) (charts []HelmChart) {
	registry := getAlibabaOCIRegistry()
	if registry == "" {
		ginkgo.GinkgoLogr.Info("ALIBABA_OPERATOR_REGISTRY environment variable is not set; cannot list chart versions")
		return
	}

	chartRef := fmt.Sprintf("%s/%s", registry, chartName)

	// Probe chart major versions 108-112 (corresponding to Rancher 2.13-2.17)
	for major := 112; major >= 108; major-- {
		versionConstraint := fmt.Sprintf("~%d", major)
		version := fetchOCIChartVersion(chartRef, versionConstraint)
		if version != "" {
			charts = append(charts, HelmChart{
				Name:           chartName,
				DerivedVersion: version,
			})
		}
	}
	ginkgo.GinkgoLogr.Info(fmt.Sprintf("Alibaba chart versions found: %v", charts))
	return
}

// VersionCompare compares Versions v to o:
// -1 == v is less than o
// 0 == v is equal to o
// 1 == v is greater than o
func VersionCompare(v, o string) int {
	ginkgo.GinkgoLogr.Info(fmt.Sprintf("Version %s vs %s", v, o))
	latestVer, err := semver.ParseTolerant(v)
	Expect(err).To(BeNil())
	oldVer, err := semver.ParseTolerant(o)
	Expect(err).To(BeNil())
	return latestVer.Compare(oldVer)
}

// GetAlibabaChartVersionForRancher fetches the appropriate Alibaba chart version for the current Rancher version.
// It queries the OCI registry for the chart version matching the Rancher minor version.
// Chart versions follow the pattern: 108.x.x+up1.13.x (for Rancher 2.13), 109.x.x+up1.14.x (for Rancher 2.14), etc.
// The chart major version = Rancher minor version + 95 (e.g., 2.13 → 108, 2.14 → 109).
func GetAlibabaChartVersionForRancher() string {
	registry := getAlibabaOCIRegistry()
	if registry == "" {
		ginkgo.GinkgoLogr.Info("ALIBABA_OPERATOR_REGISTRY environment variable is not set; cannot auto-detect chart version")
		return ""
	}

	_, rancherVersion, rancherHeadVersion := GetRancherVersions(RancherFullVersion)

	ver := rancherVersion
	if rancherHeadVersion != "" {
		ver = rancherHeadVersion
	}

	parsedVer, err := semver.ParseTolerant(ver)
	if err != nil {
		ginkgo.GinkgoLogr.Info(fmt.Sprintf("Unable to parse Rancher version %q: %v", ver, err))
		return ""
	}

	// Calculate chart major version: Rancher minor + 95 (e.g., 2.13 → 108, 2.14 → 109)
	chartMajor := int(parsedVer.Minor) + 95

	ginkgo.GinkgoLogr.Info(fmt.Sprintf("Rancher version: %s, chart major version: %d", ver, chartMajor))

	chartRef := fmt.Sprintf("%s/rancher-ali-operator", registry)
	versionConstraint := fmt.Sprintf("~%d", chartMajor)

	version := fetchOCIChartVersion(chartRef, versionConstraint)
	if version != "" {
		ginkgo.GinkgoLogr.Info(fmt.Sprintf("Found matching chart version %s for Rancher %s", version, ver))
	}
	return version
}

// getAlibabaOCIRegistry returns the Alibaba OCI registry URL from the ALIBABA_OPERATOR_REGISTRY env var.
func getAlibabaOCIRegistry() string {
	return os.Getenv("ALIBABA_OPERATOR_REGISTRY")
}

// fetchOCIChartVersion runs helm show chart against an OCI reference with a version constraint
// and returns the chart version, or empty string on failure.
func fetchOCIChartVersion(chartRef, versionConstraint string) string {
	cmd := exec.Command("helm", "show", "chart", chartRef, "--version", versionConstraint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		}
	}
	return ""
}
