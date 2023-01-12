# This test verifies prometheus metrics from the dynamic-pools policy.

cleanup() {
    vm-command "kubectl delete pods --all --now"
    terminate cri-resmgr
    terminate cri-resmgr-agent
    vm-command "cri-resmgr -reset-policy; cri-resmgr -reset-config"
    return 0
}

cleanup

# Launch cri-resmgr with wanted metrics update interval and a
# configuration that opens the instrumentation http server.
cri_resmgr_cfg=${TEST_DIR}/dyp-metrics.cfg  cri_resmgr_extra_args="-metrics-interval 1s" launch cri-resmgr
sleep 10
verify-metrics-has-line 'dynamicPool="shared"'
verify-metrics-has-line 'dynamicPool="reserved"'
verify-metrics-has-line 'dynamicPool="full-core"'
verify-metrics-has-line 'dynamicPool="flex"'
verify-metrics-has-line 'dynamicPool="fast-dualcore"'

# pod0: run in shared dynamic pool.
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
CONTCOUNT=2 create dyp-busybox
report allowed
verify-metrics-has-line 'dynamicPool="reserved"'
verify-metrics-has-line 'dynamicPool="full-core"'
verify-metrics-has-line 'dynamicPool="flex"'
verify-metrics-has-line 'dynamicPool="fast-dualcore"'
verify-metrics-has-line 'DynamicPools{containers="pod0:pod0c0,pod0:pod0c1",cpu_class="",cpus=".*",dynamicPool="shared",dynamicPool_type="shared",mems=".*",tot_limit_millicpu="200",tot_req_millicpu="200"} 15'

# pod1: run in fast-dualcore dynamic pool.
CPUREQ="200m" MEMREQ="" CPULIM="200m" MEMLIM=""
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: fast-dualcore" CONTCOUNT=1 create dyp-busybox
report allowed
verify-metrics-has-line 'containers="pod1:pod1c0".*dynamicPool="fast-dualcore",dynamicPool_type="fast-dualcore".*tot_req_millicpu="(199|200)"'
verify 'len(cpus["pod1c0"]) >= 1'

# pod2: run in flex dynamic pool.
CPUREQ="3500m" MEMREQ="" CPULIM="3500m" MEMLIM=""
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: flex" CONTCOUNT=1 create dyp-busybox
report allowed
verify-metrics-has-line 'containers="pod2:pod2c0".*dynamicPool="flex",dynamicPool_type="flex"'
verify 'len(cpus["pod2c0"]) >= 4'

# pod3: run in flex dynamic pool.
CPUREQ="1200m" MEMREQ="" CPULIM="1200m" MEMLIM=""
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: flex" CONTCOUNT=1 create dyp-busybox
report allowed
verify-metrics-has-line 'containers="pod2:pod2c0,pod3:pod3c0".*dynamicPool="flex",dynamicPool_type="flex"'
verify 'len(cpus["pod2c0"]) >= 5'

# Resize flex dynamic pool in metrics.
kubectl delete pods --now pod3
verify-metrics-has-line 'containers="pod2:pod2c0".*dynamicPool="flex",dynamicPool_type="flex"'
verify 'len(cpus["pod2c0"]) >= 4'

kubectl delete pods --now pod2
sleep 5
verify-metrics-has-line 'containers="".*dynamicPool="flex",dynamicPool_type="flex".*0'

# Delete all pods in shared dynamic pool.
kubectl delete pods --now pod0
# pod4: run in fast-dualcore dynamic pool, all CPUs are allocated to fast-dualcore dynamic pool.
CPUREQ="14" MEMREQ="" CPULIM="14" MEMLIM=""
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: fast-dualcore" CONTCOUNT=1 create dyp-busybox
report allowed
verify-metrics-has-line 'containers="pod1:pod1c0,pod4:pod4c0".*dynamicPool="fast-dualcore",dynamicPool_type="fast-dualcore".*15'
verify 'len(cpus["pod1c0"]) == 15'

cleanup
