# Test that the static-pools policy
# 1. labels the node with cmk.intel.com/cmk-node
# 2. advertises correct number of exclusive-cores resources
# 3. taints the node

# shellcheck disable=SC2148
cri_resmgr_cfg="$TEST_DIR/../cri-resmgr-static-pools.cfg" static-pools-relaunch-cri-resmgr

out ""
out "### Verifying that node has cmk-node label"
vm-run-until 'kubectl get nodes -o jsonpath="{.items[*].metadata.labels}" | grep \"cmk.intel.com/cmk-node\"\:\"true\"' ||
    error "cmk.intel.com/cmk-node label missing"

out ""
out "### Verifying that amount exclusive cores on node matches /etc/cmk/pools.conf"
vm-run-until 'kubectl get nodes -o jsonpath="{.items[*].status.allocatable}" | grep -q \"cmk.intel.com/exclusive-cores\"\:\"3\"' ||
    error "expected 3 allocatable cmk.intel.com/exclusive-cores"

out ""
out "### Creating a pod that should not be scheduled due to node taint"
( wait_t=2s create besteffort ) || {
    echo "failed as expected due to node taint"
}

out ""
out "### Verifying that scheduling normal pod failed"
vm-command 'kubectl describe pods/pod0 | grep -E "FailedScheduling .*cmk: true"' || {
    error "FailedScheduling expected but not found"
}
