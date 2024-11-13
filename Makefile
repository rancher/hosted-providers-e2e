##################
### USED BY CI ###
##################

STANDARD_TEST_OPTIONS= -v -r --timeout=3h --keep-going --randomize-all --randomize-suites
BUILD_DATE= $(shell date +'%Y%m%d')

prepare-rancher: deps install-helm ## Install k3s and Rancher with dependencies on the local machine
	ginkgo --label-filter install -r -v ./hosted/preparation

install-helm: ## Install latest Helm on the local machine
	curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

install-rancher-hosted-nightly-chart: prepare-rancher ## Install Rancher via Helm with hosted providers nightly chart
	helm upgrade --install ${PROVIDER}-operator-crds  oci://ttl.sh/${PROVIDER}-operator/rancher-${PROVIDER}-operator-crd --version ${BUILD_DATE}
	helm upgrade --install ${PROVIDER}-operator oci://ttl.sh/${PROVIDER}-operator/rancher-${PROVIDER}-operator --version ${BUILD_DATE} --namespace cattle-system

deps: ## Install the Go dependencies
	go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo
	go install -mod=mod github.com/onsi/gomega
	go mod tidy

prepare-e2e-ci-rancher-hosted-nightly-chart: prepare-rancher install-rancher-hosted-nightly-chart ## Setup Rancher with nightly hosted provider charts on the local machine
prepare-e2e-ci-rancher: prepare-rancher ## Setup Rancher on the local machine
prepare-e2e-ci-rancher-behind-proxy: prepare-rancher ## Setup Rancher behind proxy on the local machine
prepare-e2e-ci-rancher-hosted-nightly-chart-behind-proxy: prepare-rancher install-rancher-hosted-nightly-chart ## Setup Rancher with nightly hosted provider charts behind proxy on the local machine

e2e-import-tests: deps	## Run the 'P0Import' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --nodes 2 --focus "P0Import" ./hosted/${PROVIDER}/p0/

e2e-provisioning-tests: deps ## Run the 'P0Provisioning' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --nodes 2 --focus "P0Provisioning" ./hosted/${PROVIDER}/p0/

e2e-p1-import-tests: deps	## Run the 'P1Import' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --nodes 2 --focus "P1Import" ./hosted/${PROVIDER}/p1/

e2e-p1-provisioning-tests: deps ## Run the 'P1Provisioning' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --focus "P1Provisioning" ./hosted/${PROVIDER}/p1/

e2e-sync-import-tests: deps ## Run "SyncImport" test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --nodes 2 --focus "SyncImport" ./hosted/${PROVIDER}/p1

e2e-sync-provisioning-tests: deps ## Run "SyncProvisioning" test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --nodes 2 --focus "SyncProvisioning" ./hosted/${PROVIDER}/p1

e2e-support-matrix-import-tests: deps ## Run the 'SupportMatrixImport' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --nodes 2 --focus "SupportMatrixImport" ./hosted/${PROVIDER}/support_matrix/

e2e-support-matrix-provisioning-tests: deps ## Run the 'SupportMatrixProvisioning' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --nodes 2 --focus "SupportMatrixProvisioning" ./hosted/${PROVIDER}/support_matrix/

e2e-k8s-chart-support-import-tests-upgrade: deps ## Run the 'K8sChartSupportUpgradeImport' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --focus "K8sChartSupportUpgradeImport" ./hosted/${PROVIDER}/k8s_chart_support/upgrade

e2e-k8s-chart-support-provisioning-tests-upgrade: deps ## Run the 'K8sChartSupportUpgradeProvisioning' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --focus "K8sChartSupportUpgradeProvisioning" ./hosted/${PROVIDER}/k8s_chart_support/upgrade

e2e-k8s-chart-support-import-tests: deps ## Run the 'K8sChartSupportImport' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --focus "K8sChartSupportImport" ./hosted/${PROVIDER}/k8s_chart_support

e2e-k8s-chart-support-provisioning-tests: deps ## Run the 'K8sChartSupportProvisioning' test suite for a given ${PROVIDER}
	ginkgo ${STANDARD_TEST_OPTIONS} --focus "K8sChartSupportProvisioning" ./hosted/${PROVIDER}/k8s_chart_support

clean-k3s:	## Uninstall k3s cluster
	/usr/local/bin/k3s-killall.sh && /usr/local/bin/k3s-uninstall.sh || true
	sudo rm -r /etc/default/k3s || true

clean-all: clean-k3s	## Cleanup the Helm repo
	/usr/local/bin/helm repo remove rancher-latest jetstack || true
	docker stop squid_proxy || true
	docker rm squid_proxy || true

########################
### LOCAL DEPLOYMENT ###
########################

help: ## Show this Makefile's help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
