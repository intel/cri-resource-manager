# Test that AvailableResources are honored.

# Test explicit cpuset in AvailableResources.CPU
terminate cri-resmgr
AVAILABLE_CPU="cpuset:4-7,8-11"
cri_resmgr_cfg=$(instantiate cri-resmgr-available-resources.cfg)
launch cri-resmgr

# pod0: exclusive CPUs
CPU=3 create guaranteed
verify "cpus['pod0c0'] == {'cpu04', 'cpu05', 'cpu06'}" \
       "mems['pod0c0'] == {'node1'}"

# pod1: shared CPUs
CONTCOUNT=2 CPU=980m create guaranteed
verify "cpus['pod1c0'] == {'cpu08', 'cpu09', 'cpu10'}" \
       "cpus['pod1c1'] == {'cpu08', 'cpu09', 'cpu10'}" \
       "mems['pod1c0'] == {'node2'}" \
       "mems['pod1c1'] == {'node2'}"
kubectl delete pods --all --now
reset counters

# Test cgroup cpuset directory in AvailableResources.CPU

test-and-verify-allowed() {
    # pod0: shared CPUs
    CONTCOUNT=2 CPU=980m create guaranteed
    report allowed
    verify "cpus['pod0c0'] == {'cpu0$1', 'cpu0$2', 'cpu0$3'}" \
           "cpus['pod0c1'] == {'cpu0$4'}"

    # pod1: exclusive CPU
    CPU=1 create guaranteed
    report allowed
    verify "disjoint_sets(cpus['pod1c0'], cpus['pod0c0'])" \
           "disjoint_sets(cpus['pod1c0'], cpus['pod0c1'])"

    kubectl delete pods --all --now
    reset counters
}

CRIRM_CGROUP=/sys/fs/cgroup/cpuset/cri-resmgr-test-05-1
vm-command "rm -rf $CRIRM_CGROUP; mkdir $CRIRM_CGROUP; echo 1-4,11 > $CRIRM_CGROUP/cpuset.cpus"

terminate cri-resmgr
AVAILABLE_CPU="\"$CRIRM_CGROUP\""
cri_resmgr_cfg=$(instantiate cri-resmgr-available-resources.cfg)
launch cri-resmgr
test-and-verify-allowed 1 2 3 4
vm-command "rmdir $CRIRM_CGROUP || true"

CRIRM_CGROUP=/sys/fs/cgroup/cpuset/cri-resmgr-test-05-2
vm-command "rm -rf $CRIRM_CGROUP; mkdir $CRIRM_CGROUP; echo 5-8,11 > $CRIRM_CGROUP/cpuset.cpus"

terminate cri-resmgr
AVAILABLE_CPU="\"${CRIRM_CGROUP#/sys/fs/cgroup/cpuset}\""
cri_resmgr_cfg=$(instantiate cri-resmgr-available-resources.cfg)
launch cri-resmgr
test-and-verify-allowed 5 6 7 8
vm-command "rmdir $CRIRM_CGROUP || true"

# cleanup, do not leave weirdly configured cri-resmgr running
terminate cri-resmgr
