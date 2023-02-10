# This test verifies that configuration updates via cri-resmgr-agent
# are handled properly in the dynamic-pools policy.

testns=e2e-dyp-test06

cleanup() {
    vm-command "kubectl delete pods --all --now; \
        kubectl delete pods -n $testns --all --now; \
        kubectl delete pods -n dyptype1ns0 --all --now; \
        kubectl delete namespace $testns || :; \
        kubectl delete namespace dyptype1ns0 || :"
    terminate cri-resmgr
    terminate cri-resmgr-agent
    vm-command "cri-resmgr -reset-policy; cri-resmgr -reset-config"
}

apply-configmap() {
    vm-put-file $(instantiate dyp-configmap.yaml) dyp-configmap.yaml
    vm-command "cat dyp-configmap.yaml"
    kubectl apply -f dyp-configmap.yaml
}

cleanup
cri_resmgr_extra_args="-metrics-interval 1s" cri_resmgr_config=fallback launch cri-resmgr
launch cri-resmgr-agent

kubectl create namespace $testns
kubectl create namespace dyptype1ns0

AVAILABLE_CPU="cpuset:1,4-15" DYPTYPE2_NAMESPACE0='"*"' apply-configmap
sleep 3

# pod0 run in dyptype0, annotation
CPUREQ=1 MEMREQ="100M" CPULIM=1 MEMLIM="100M"
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: dyptype0" create dyp-busybox
# pod1 run in dyptype1, namespace
CPUREQ=1 MEMREQ="100M" CPULIM=1 MEMLIM="100M"
namespace="dyptype1ns0" create dyp-busybox
# pod2 run in dyptype2, wildcard namespace
CPUREQ=1 MEMREQ="100M" CPULIM=1 MEMLIM="100M"
namespace="e2e-dyp-test06" create dyp-busybox
sleep 3
vm-command "curl -s $verify_metrics_url"
verify-metrics-has-line 'pod0:pod0c0.*"dyptype0"'
verify-metrics-has-line 'pod1:pod1c0.*"dyptype1"'
verify-metrics-has-line 'pod2:pod2c0.*"dyptype2"'

# Remove first two dynamic pool types, change dyptype2 to match all
# namespaces.
DYPTYPE0_SKIP=1 DYPTYPE1_SKIP=1 DYPTYPE2_NAMESPACE0='"*"' apply-configmap
# Note:

# pod0 was successfully assigned to and running in dyptype0 dynamic pool.
# Now dyptype0 was completely removed from the node.
# Currently this behavior is undefined.
# Possible behaviors: evict pod0, continue assign chain, refuse config...
# For now, skip pod0c0 dynamic pool validation:
# verify-metrics-has-line '"dyptype2".*pod0:pod0c0'
verify-metrics-has-line 'pod1:pod1c0.*"dyptype2"'
verify-metrics-has-line 'pod2:pod2c0.*"dyptype2"'

# Bring back dyptype0 where pod0 belongs to by annotation.
DYPTYPE1_SKIP=1 DYPTYPE2_NAMESPACE0='"*"' apply-configmap
verify-metrics-has-line 'pod0:pod0c0.*"dyptype0"'
verify-metrics-has-line 'pod1:pod1c0.*"dyptype2"'
verify-metrics-has-line 'pod2:pod2c0.*"dyptype2"'

# Change only CPU classes, no reassigning.
verify-metrics-has-line 'pod0:pod0c0.*cpu_class="classA".*"dyptype0"'
verify-metrics-has-line 'pod1:pod1c0.*cpu_class="classC".*"dyptype2"'
verify-metrics-has-line 'pod2:pod2c0.*cpu_class="classC".*"dyptype2"'
DYPTYPE0_CPUCLASS="classC" DYPTYPE1_SKIP=1 DYPTYPE2_CPUCLASS="classB" DYPTYPE2_NAMESPACE0='"*"'  apply-configmap
verify-metrics-has-line 'pod0:pod0c0.*cpu_class="classC".*"dyptype0"'
verify-metrics-has-line 'pod1:pod1c0.*cpu_class="classB".*"dyptype2"'
verify-metrics-has-line 'pod2:pod2c0.*cpu_class="classB".*"dyptype2"'

cleanup
launch cri-resmgr
