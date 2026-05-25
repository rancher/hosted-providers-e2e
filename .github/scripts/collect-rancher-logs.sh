#!/bin/bash

set -evx

# Variables
RANCHER_SUPPORT_TOOLS_VERSION="40ce9841dcbd82697548229b31aafcfd3a6bd1fd"

RANCHER_LOG_COLLECTER="https://raw.githubusercontent.com/rancherlabs/support-tools/${RANCHER_SUPPORT_TOOLS_VERSION}/collection/rancher/v2.x/logs-collector/rancher2_logs_collector.sh"

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

# Check if proxy has been implemented then fetch proxy logs
if [[ $RANCHER_BEHIND_PROXY = true ]]; then
  mkdir -p -m 755 proxy-logs
  cd proxy-logs
  docker cp squid_proxy:/var/log/squid/access.log ./squid-access.log
fi
# Done!
exit 0
