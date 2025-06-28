#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

source /helpers.sh
source /test-constants.sh

for VERSION in ${SUPPORTED_VERSIONS}
do
  if nodeadm install $VERSION --credential-provider ssm --download-timeout 1s; then
    echo "install should not succeed in 1 second"
    exit 1
  fi
done
