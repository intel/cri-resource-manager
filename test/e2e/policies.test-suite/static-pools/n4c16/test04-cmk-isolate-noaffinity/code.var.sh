# Test that cmk isolate --no-affinity is effective on every pool
# with and without STP_POOL / STP_SOCKET_ID env vars.
# Test that all exclusive cores can be consumed with --no-affinity.

# shellcheck disable=SC2148
cri_resmgr_cfg="$TEST_DIR/../cri-resmgr-static-pools.cfg" static-pools-relaunch-cri-resmgr
export STP_POOL="" STP_SOCKET_ID="" CMDSPLIT="command_all"
export ECHO_VARS='CMK_CPUS_ASSIGNED="$CMK_CPUS_ASSIGNED" CMK_CPUS_SHARED="$CMK_CPUS_SHARED" CMK_CPUS_INFRA="$CMK_CPUS_INFRA"'

export CMK_ISOLATE=", '--conf-dir=/etc/cmk.conf', '--pool=exclusive', '--socket-id=0', '--no-affinity'"
out ""
out "### Creating no-affinity pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES=1 create cmk-isolate
report allowed
verify 'len(cores["pod0c0"]) == 8'
cpus_assigned="$(kubectl logs pod0 | tail -n 1 | awk '{print $2}')"
cpus_shared="$(kubectl logs pod0 | tail -n 1 | awk '{print $3}')"
cpus_infra="$(kubectl logs pod0 | tail -n 1 | awk '{print $4}')"
[ "$cpus_assigned" == "CMK_CPUS_ASSIGNED=0,1" ] ||
    error "expected CMK_CPUS_ASSIGNED=0,1, got $cpus_assigned"
[ "$cpus_shared" == "CMK_CPUS_SHARED=4-6,7" ] ||
    error "expected CMK_CPUS_SHARED=4-6,7, got $cpus_shared"
[ "$cpus_infra" == "CMK_CPUS_INFRA=10-15" ] ||
    error "expected CMK_CPUS_INFRA=10-15, got $cpus_infra"

export CMK_ISOLATE=", '--conf-dir=/etc/cmk.conf', '--pool=exclusive', '--socket-id=1', '--no-affinity'"
out ""
out "### Creating no-affinity pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES=1 STP_POOL="exclusive" STP_SOCKET_ID="1" create cmk-isolate
report allowed
verify 'len(cores["pod1c0"]) == 8'
cpus_assigned="$(kubectl logs pod1 | tail -n 1 | awk '{print $2}')"
[ "$cpus_assigned" == "CMK_CPUS_ASSIGNED=8,9" ] ||
    error "expected CMK_CPUS_ASSIGNED=8,9, got $cpus_assigned"

export CMK_ISOLATE=", '--conf-dir=/etc/cmk.conf', '--pool=exclusive', '--no-affinity'"
out ""
out "### Creating no-affinity pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES=1 STP_POOL="exclusive" create cmk-isolate
report allowed
verify 'len(cores["pod2c0"]) == 8'
cpus_assigned="$(kubectl logs pod2 | tail -n 1 | awk '{print $2}')"
[ "$cpus_assigned" == "CMK_CPUS_ASSIGNED=2,3" ] ||
    error "expected CMK_CPUS_ASSIGNED=2,3, got $cpus_assigned"

export CMK_ISOLATE=", '--no-affinity', '--pool=shared'"
out ""
out "### Creating no-affinity pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES="" create cmk-isolate
report allowed
verify 'len(cores["pod3c0"]) == 8'
cpus_assigned="$(kubectl logs pod3 | tail -n 1 | awk '{print $2}')"
[ "$cpus_assigned" == "CMK_CPUS_ASSIGNED=4-6,7" ] ||
    error "expected CMK_CPUS_ASSIGNED=5-6,7, got $cpus_assigned"

export CMK_ISOLATE=", '--pool=infra', '--no-affinity'"
out ""
out "### Creating no-affinity pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES="" create cmk-isolate
report allowed
verify 'len(cores["pod4c0"]) == 8'
cpus_assigned="$(kubectl logs pod4 | tail -n 1 | awk '{print $2}')"
[ "$cpus_assigned" == "CMK_CPUS_ASSIGNED=10-15" ] ||
    error "expected CMK_CPUS_ASSIGNED=10-15 got $cpus_assigned"
