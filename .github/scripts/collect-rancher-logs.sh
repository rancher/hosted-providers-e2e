#!/bin/bash

set -evx

# Variables
RANCHER_LOG_COLLECTER="https://raw.githubusercontent.com/rancherlabs/support-tools/master/collection/rancher/v2.x/logs-collector/rancher2_logs_collector.sh"
CRUST_GATHER_INSTALLER="https://github.com/crust-gather/crust-gather/raw/main/install.sh"

# Create directory to store logs
mkdir -p -m 755 logs
cd logs


# Download and run the log collector script
mkdir -p -m 755 cluster-logs
cd cluster-logs
curl -L ${RANCHER_LOG_COLLECTER} -o rancherlogcollector.sh
chmod +x rancherlogcollector.sh
sudo ./rancherlogcollector.sh -d ../cluster-logs

# Move back to logs dir
cd ..

# Download, install and run the crust-gather script
mkdir -p -m 755 crust-gather-logs
cd crust-gather-logs

curl -L ${CRUST_GATHER_INSTALLER} -o crust-gather-installer.sh
chmod +x crust-gather-installer.sh
sudo ./crust-gather-installer.sh -y

crust-gather collect

cat > USAGE.md <<EOF
To use crust-gather; do the following:
1. Run the command 'crust-gather-installer.sh -y' or download it from $CRUST_GATHER_INSTALLER
2. 'touch kubeconfig'
3. 'export KUBECONFIG=kubeconfig'
4. Start the server in backgroud on a port: 'crust-gather serve --socket 127.0.0.1:8089 &'
5. Check the content of kubeconfig file: 'cat kubeconfig'
6. Run any kubectl command to check if it works: 'kubectl get pods -A'.
EOF


# Done!
exit 0
