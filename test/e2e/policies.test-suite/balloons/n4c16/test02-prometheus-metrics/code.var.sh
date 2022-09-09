# This test verifies prometheus metrics from the balloons policy.

cleanup() {
    vm-command "kubectl delete pods --all --now"
    return 0
}

cleanup

# Launch cri-resmgr with wanted metrics update interval and a
# configuration that opens the instrumentation http server.
terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-metrics.cfg  cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr
verify-metrics-has-line 'balloon="default\[0\]"'
verify-metrics-has-line 'balloon="reserved\[0\]"'
verify-metrics-has-no-line 'balloon="full-core\[0\]"'
verify-metrics-has-no-line 'balloon_type="full-core"'
verify-metrics-has-no-line 'balloon_type="fast-dualcore"'
verify-metrics-has-no-line 'balloon_type="flex"'

# pod0 in full-core[0]
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: full-core" CONTCOUNT=2 create balloons-busybox
report allowed
verify-metrics-has-line 'balloon="default\[0\]"'
verify-metrics-has-line 'balloon="reserved\[0\]"'
verify-metrics-has-line 'balloons{balloon="full-core\[0\]",balloon_type="full-core",containers="pod0:pod0c0,pod0:pod0c1",cpu_class="normal",cpus="2-3",cpus_max="2",cpus_min="2",mems="0",tot_req_millicpu="(199|200)"} 2'

# pod1 in fast-dualcore[0]
CPUREQ="200m" MEMREQ="" CPULIM="200m" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: fast-dualcore" CONTCOUNT=1 create balloons-busybox
report allowed
verify-metrics-has-line 'balloon="fast-dualcore\[0\]".*tot_req_millicpu="(199|200)".* 4'
verify-metrics-has-no-line 'balloon="fast-dualcore\[1\]"'

# pod2 in fast-dualcore[1] (FillChain prefers new-balloon)
CPUREQ="500m" MEMREQ="" CPULIM="500m" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: fast-dualcore" CONTCOUNT=1 create balloons-busybox
report allowed
verify-metrics-has-line 'balloon="fast-dualcore\[0\]"'
verify-metrics-has-line 'balloon="fast-dualcore\[1\]".*tot_req_millicpu="500".* 4'
verify-metrics-has-no-line 'balloon_type="flex"'

# pod3 in flex[0]
CPUREQ="3500m" MEMREQ="" CPULIM="3500m" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: flex" CONTCOUNT=1 create balloons-busybox
report allowed
verify-metrics-has-line 'balloon_type="flex".* 4'

# pod4 in flex[0], balloon inflated to fit pod3 + pod4
CPUREQ="1200m" MEMREQ="" CPULIM="1200m" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: flex" CONTCOUNT=1 create balloons-busybox
report allowed
verify-metrics-has-line 'balloon_type="flex"'
verify-metrics-has-line 'balloon_type="flex".* 5'

# check deflating a balloon in metrics
kubectl delete pods --now pod3
verify-metrics-has-line 'balloon_type="flex"'
verify-metrics-has-line 'balloon_type="flex".* 2'

kubectl delete pods --now pod4
sleep 5
# check popping a balloon from metrics
verify-metrics-has-no-line 'balloon_type="flex"'

# pop fast-dualcore[0], keep fast-dualcore[1]
kubectl delete pods --now pod1
verify-metrics-has-line 'balloon="fast-dualcore\[1\]"'
sleep 5
verify-metrics-has-no-line 'balloon="fast-dualcore\[0\]"'

# re-create balloon instance fast-dualcore[0] that was just popped.
# pod5 in fast-dualcore[0], pod2 keeps running in fast-dualcore[1]
CPUREQ="4000m" MEMREQ="100M" CPULIM="4000m" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: fast-dualcore" CONTCOUNT=1 create balloons-busybox
report allowed
verify-metrics-has-line 'balloon="fast-dualcore\[1\]".*pod2c0.* 4'
verify-metrics-has-line 'balloon="fast-dualcore\[0\]".*pod5c0.* 4'

# # Re-launch cri-resmgr with test suite default parameters
# terminate cri-resmgr
# launch cri-resmgr

cleanup
