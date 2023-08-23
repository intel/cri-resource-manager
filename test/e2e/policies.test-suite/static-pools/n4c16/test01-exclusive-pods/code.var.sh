# Test that exclusive-cores containers
# 1. run on exclusive cores
# 2. are pinned according to STP_POOL and STP_SOCKET_ID
#    when "cmk isolate" is not used.
# 3. all exclusive cores can be consumed with and without
#    specifying STP_SOCKET_ID.

# shellcheck disable=SC2148
cri_resmgr_cfg="$TEST_DIR/../cri-resmgr-static-pools.cfg" static-pools-relaunch-cri-resmgr
export STP_POOL=exclusive

out ""
out "### Creating exclusive CMK pod with 1 exclusive core"
CPU=1000m STP_SOCKET_ID=1 EXCLCORES=1 create cmk-exclusive
report allowed
verify 'len(cores["pod0c0"]) == 1' \
       'packages["pod0c0"] == {"package1"}'

out ""
out "### Deleting exclusive CMK pod"
kubectl delete pods --all --now --wait

out ""
out "### Creating exclusive CMK pod with 2 exclusive cores"
CPU=1000m STP_SOCKET_ID=0 EXCLCORES=2 create cmk-exclusive
report allowed
verify 'len(cores["pod1c0"]) == 2' \
       'packages["pod1c0"] == {"package0"}'

out ""
out "### Deleting exclusive CMK pod"
kubectl delete pods --all --now --wait

out ""
out "### Creating two exclusive CMK pods with 1 exclusive core each"
n=2 CPU=1000m STP_SOCKET_ID=0 EXCLCORES=1 create cmk-exclusive
report allowed
verify 'len(cores["pod2c0"]) == 1' \
       'len(cores["pod3c0"]) == 1' \
       'disjoint_sets(cores["pod2c0"], cores["pod3c0"])' \
       'packages["pod2c0"] == packages["pod3c0"] == {"package0"}'

out ""
out "### Creating one more exclusive CMK pods, consuming all exclusive cores"
CPU=1000m STP_SOCKET_ID=1 EXCLCORES=1 create cmk-exclusive
report allowed
verify 'len(cores["pod2c0"]) == 1' \
       'len(cores["pod3c0"]) == 1' \
       'len(cores["pod4c0"]) == 1' \
       'disjoint_sets(cores["pod2c0"], cores["pod3c0"], cores["pod4c0"])' \
       'set.union(cores["pod2c0"], cores["pod3c0"], cores["pod4c0"]) == exclusive_cores'
kubectl delete pods --all --now --wait

out ""
out "### Test consuming all exclusive cores without specifying STP_SOCKET_ID"
n=3 CPU=1000m STP_SOCKET_ID="" EXCLCORES=1 create cmk-exclusive
verify 'len(cores["pod5c0"]) == 1' \
       'len(cores["pod6c0"]) == 1' \
       'len(cores["pod7c0"]) == 1' \
       'disjoint_sets(cores["pod5c0"], cores["pod6c0"], cores["pod7c0"])' \
       'set.union(cores["pod5c0"], cores["pod6c0"], cores["pod7c0"]) == exclusive_cores'
