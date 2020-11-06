# Test that legacy exclusive-cores containers
# 1. run on exclusive cores
# 2. are pinned according to "cmk isolate" command
#    parameters
# 3. run without "cmk" existing on the image
# 3. all exclusive cores can be consumed

# shellcheck disable=SC2148
cri_resmgr_cfg="$TEST_DIR/../cri-resmgr-static-pools.cfg" static-pools-relaunch-cri-resmgr
export STP_POOL="" STP_SOCKET_ID=""

export CMK_ISOLATE=", '--conf-dir=/etc/cmk.conf', '--pool=exclusive', '--socket-id=1'"
out ""
out "### Creating pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES=1 CMDSPLIT="command_all" create cmk-isolate
report allowed
verify 'len(cores["pod0c0"]) == 1' \
       'cores["pod0c0"].issubset(exclusive_cores)' \
       'packages["pod0c0"] == {"package1"}'

export CMK_ISOLATE=", '--socket-id=0', '--pool=exclusive'"
out ""
out "### Creating pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=2000m EXCLCORES=2 CMDSPLIT="command_cmk_sh" create cmk-isolate
report allowed
verify 'len(cores["pod1c0"]) == 2' \
       'cores["pod1c0"].issubset(exclusive_cores)' \
       'packages["pod1c0"] == {"package0"}'

export CMK_ISOLATE=", '--pool=shared'"
out ""
out "### Creating pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES="" CMDSPLIT="command_cmk" create cmk-isolate
report allowed
verify 'cores["pod2c0"] == shared_cores'

export CMDSPLIT="command_cmk"

export CMK_ISOLATE=", '--conf-dir=/etc/cmk.conf', '--pool=infra'"
out ""
out "### Creating pod 'cmk', 'isolate'$CMK_ISOLATE..."
CPU=1000m EXCLCORES="" create cmk-isolate
report allowed
verify 'cores["pod3c0"] == infra_cores'

out ""
out "### Deleting only exclusive CMK pods, leave shared/infra running"
kubectl delete pods/pod0 pods/pod1 --now

export CMK_ISOLATE=", '--pool=exclusive'"
out ""
out "### Creating 3 exclusive pods 'cmk', 'isolate'$CMK_ISOLATE..."
n=3 CPU=1000m EXCLCORES=1 create cmk-isolate
report allowed
verify 'len(cores["pod4c0"]) == 1' \
       'len(cores["pod5c0"]) == 1' \
       'len(cores["pod6c0"]) == 1' \
       'disjoint_sets(cores["pod4c0"], cores["pod5c0"], cores["pod6c0"])' \
       'cores["pod4c0"].issubset(exclusive_cores)' \
       'cores["pod5c0"].issubset(exclusive_cores)' \
       'cores["pod6c0"].issubset(exclusive_cores)'
