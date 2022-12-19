# Test reporting Prometheus metrics from podpools

cleanup() {
    vm-command "kubectl get pods -A | grep -E ' pod[0-9]' | while read namespace pod rest; do kubectl -n \$namespace delete pod \$pod --now; done"
}

parse-commandoutput-log_pool_cpuset() {
    log_pool_cpuset=$(awk -F 'cpus:|, ' "{print \$2}" <<< "$COMMAND_OUTPUT")
    out "parsed: log_pool_cpuset=$log_pool_cpuset"
}

parse-commandoutput-log_pool_name() {
    log_pool_name=$(awk -F"[ {]*" "{print \$10}" <<< "$COMMAND_OUTPUT")
    out "parsed: log_pool_name=$log_pool_name"
}

verify-log-vs-metrics() {
    local podXcY="$1"
    local cpuUsageMin="$2" # optional
    local cpuUsageMax="$3" # optional
    vm-command "grep 'assigning container $podXcY to pool' cri-resmgr.output.txt"
    parse-commandoutput-log_pool_cpuset
    parse-commandoutput-log_pool_name
    local usageCmd="curl --silent $metrics_url | grep $log_pool_cpuset | grep $podXcY"
    vm-run-until --timeout 10 "$usageCmd" || {
        error "cannot find pod:container $1 and cpuset $log_pool_cpuset from the report"
    }
    if [ -n "$cpuUsageMax" ]; then
        echo "verifying CPU usage $cpuUsageMin < X < $cpuUsageMax"
        vm-run-until --timeout 20 "X=\"\$($usageCmd)\"; echo \"\$X\"; X=\${X##* }; X=\${X%%.*}; echo $cpuUsageMin \< \$X \< $cpuUsageMax; (( $cpuUsageMin < \$X )) && (( \$X < $cpuUsageMax ))"
    fi
}

verify-metrics-has-line() {
    local expected_line="$1"
    out "verifying metrics line syntax..."
    vm-run-until --timeout 10 "echo '    waiting for metrics line: $expected_line' >&2; curl --silent $metrics_url | grep -E '$expected_line'" || {
        command-error "expected line '$1' missing from the output"
    }
}

# Delete left-over test pods from the kube-system namespace
for podX in $(kubectl get pods -n kube-system | awk '/^pod[0-9]/{print $1}'); do
    kubectl delete pods $podX -n kube-system --now
done

metrics_url="http://localhost:8891/metrics"

# Launch cri-resmgr with wanted metrics update interval
# and configuration that opens the instrumentation http server.
terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/podpools-metrics.cfg  cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr

# pod0: single container, reserve 400m CPU, but do not use it.
out ""
out "### Idle single-container pod"
CPUREQ="400m" MEMREQ="" CPULIM="400m" MEMLIM=""
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: 400mCPU" CONTCOUNT=1 create podpools-busybox
report allowed
verify-log-vs-metrics pod0:pod0c0 0 30

# pod0: single container, reserve 400m CPU and use it.
# "yes" should show up in top with 40 % CPU consumption.
out ""
out "### Busy single-container pod"
CPUREQ="400m" MEMREQ="" CPULIM="400m" MEMLIM=""
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: 400mCPU" CONTCOUNT=1 WORK='yes>/dev/null & ' create podpools-busybox
report allowed
verify-log-vs-metrics pod1:pod1c0 30 50

out ""
out "### Idle four-container pod"
CPUREQ="100m" CPULIM="100m"
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: 400mCPU" CONTCOUNT=4 create podpools-busybox
report allowed
verify-metrics-has-line 'pool_cpu_usage{CPUs="[0-9]-[0-9]",container_name="pod2:pod2c0,pod2:pod2c1,pod2:pod2c2,pod2:pod2c3",def_name="400mCPU",memory="1",pod_name="pod2",policy="podpools",pool_size="2000",pretty_name="400mCPU\[[0-9]\]"}'
verify-log-vs-metrics pod2:pod2c3 0 30

out ""
out "### Busy four-container pod"
CPUREQ="100m" CPULIM="100m"
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: 400mCPU" CONTCOUNT=4 WORK='yes>/dev/null & ' create podpools-busybox
report allowed
verify-log-vs-metrics pod3:pod3c3 30 50

out ""
out "### Multicontainer pod, no annotations. Runs on shared CPUs."
CPUREQ="" CPULIM=""
CONTCOUNT=2 create podpools-busybox
report allowed
vm-command "curl --silent $metrics_url | grep -v ^cgroup_"
verify-log-vs-metrics pod4:pod4c1 0 30

out ""
out "### Multicontainer pod in kube-system namespace. Runs on reserved CPUs."
CPUREQ="" CPULIM=""
namespace=kube-system CONTCOUNT=3 create podpools-busybox
report allowed
vm-command "curl --silent $metrics_url | grep -v ^cgroup_"
verify-log-vs-metrics pod5:pod5c1 0 30

cleanup
