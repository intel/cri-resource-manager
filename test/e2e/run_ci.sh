#!/bin/bash

# This is executed by the Github Workflow Actions and it will run e2e tests
# with suitable parameters.

set -x

if [ -z "$1" ]; then
    echo "Usage: $0 <test-directory-to-use>"
    exit 1
fi

cd $GITHUB_WORKSPACE/test/e2e

# Set the govm cpu to be like this VM cpu.
VM_QEMU_EXTRA="-cpu host,topoext=on" k8scri="cri-resmgr|containerd" ./run_tests.sh $1
