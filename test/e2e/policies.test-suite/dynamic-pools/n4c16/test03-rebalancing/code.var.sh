# Re-launch cri-resmgr with the rebalancing parameter in order to
# enable rebalancing calls. (See help of the "launch" function for
# more options.)

cleanup() {
    vm-command "kubectl delete pods --all --now"
    return 0
}

retry() {
    local retries="$1"
    shift
    local poll_delay=1s
    while (( retries > 0 )); do
        ( "$@" ) && return 0
        retries=$(( retries - 1 ))
        echo "$retries retries left for command: $@"
        sleep "$poll_delay"
    done
    error "always failed to run: $@"
}

cleanup
terminate cri-resmgr
cri_resmgr_extra_args="-metrics-interval 1s -rebalance-interval 2s" launch cri-resmgr

# Create three pods:
# - pod0 to "shared"
# - pod1 to "pool1"
# - pod2 to "pool2"
create dyp-busybox
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: pool1"
create dyp-busybox
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: pool2"
create dyp-busybox
# Print initial CPU pinning.
report allowed
# Wait at least one rebalancing round.
sleep 3
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod1c0"]) >= 1' \
       'len(cpus["pod2c0"]) >= 1'
verify-metrics-has-line 'containers="pod0:pod0c0".*dynamicPool="shared",dynamicPool_type="shared"'
verify-metrics-has-line 'containers="pod1:pod1c0".*dynamicPool="pool1",dynamicPool_type="pool1"'
verify-metrics-has-line 'containers="pod2:pod2c0".*dynamicPool="pool2",dynamicPool_type="pool2"'

# Increase CPU usage of pod1 to 200%
vm-command "nohup kubectl exec pod1 -- /bin/sh -c 'gzip </dev/zero >/dev/null' </dev/null >&/dev/null &"
vm-command "nohup kubectl exec pod1 -- /bin/sh -c 'gzip </dev/zero >/dev/null' </dev/null >&/dev/null &"
# Wait at least one rebalancing round and print CPU pinning.
sleep 2
report allowed
# Now "pool1" has 200% CPU load, "shared" and "pool2" have 0%.
# Verify that the number of CPUs in pool1 is the largest.
retry 10 verify \
       'len(cpus["pod1c0"]) > len(cpus["pod0c0"])' \
       'len(cpus["pod1c0"]) > len(cpus["pod2c0"])' \
       'len(cpus["pod0c0"]) + len(cpus["pod1c0"]) + len(cpus["pod2c0"]) == 14'

# Remove CPU load from pool1 and put 100% CPU load to pool2.
vm-command "pkill gzip"
vm-command "nohup kubectl exec pod2 -- /bin/sh -c 'gzip </dev/zero >/dev/null' </dev/null >&/dev/null &"
# Wait at least one rebalancing round and print CPU pinning.
sleep 2
report allowed
# Verify that the number of CPUs in pool2 is the largest.
retry 10 verify \
      'len(cpus["pod2c0"]) > len(cpus["pod0c0"])' \
      'len(cpus["pod2c0"]) > len(cpus["pod1c0"])' \
      'len(cpus["pod0c0"]) + len(cpus["pod1c0"]) + len(cpus["pod2c0"]) == 14'

# Remove CPU load from pool1 and put 100% CPU load to pool2 and pool1.
vm-command "pkill gzip"
vm-command "nohup kubectl exec pod1 -- /bin/sh -c 'gzip </dev/zero >/dev/null' </dev/null >&/dev/null &"
vm-command "nohup kubectl exec pod2 -- /bin/sh -c 'gzip </dev/zero >/dev/null' </dev/null >&/dev/null &"
# Takes time to reach a state of balance
sleep 2
report allowed
# Verify that the number of CPUs in pool1 is greater than or equal to 6 and less than or equal to 8.
# Verify that the number of CPUs in pool2 is greater than or equal to 6 and less than or equal to 8.
retry 10 verify \
      'len(cpus["pod0c0"]) == 1' \
      'len(cpus["pod1c0"]) >= 6' \
      'len(cpus["pod1c0"]) <= 8' \
      'len(cpus["pod2c0"]) >= 6' \
      'len(cpus["pod2c0"]) <= 8'

# Remove CPU load from pool1 and pool2
vm-command "pkill gzip"

cleanup
