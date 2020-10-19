
# shellcheck disable=SC2148
cri_resmgr_cfg="$TEST_DIR/../cri-resmgr-static-pools.cfg" static-pools-relaunch-cri-resmgr
export STP_POOL=exclusive

errmsg_zero_cores="static-pools: exclusive pool specified but the number of exclusive CPUs requested is 0"
errmsg_non_existing_pool="static-pools: non-existent pool"
errmsg_not_enough_exclcores="static-pools: not enough free cpu lists"

out ""
out "### Request cores from non-existing pool"
( CPU=1000m STP_SOCKET_ID=0 EXCLCORES=1 STP_POOL=elusive wait_t=5s create cmk-exclusive )  &&
    error "expected timeout, but pod launched with cores from non-existing pool"
vm-run-until "kubectl describe pods/pod0 | grep '$errmsg_non_existing_pool'" ||
    error "cannot find expected error message from pod description"
out "Failed as expected"

kubectl delete pods --all --now || error "failed to delete pods"

out ""
out "### Request cores from non-existing socket"
( CPU=1000m STP_SOCKET_ID=2 EXCLCORES=1 wait_t=5s create cmk-exclusive )  &&
    error "expected timeout, but pod launched with cores from non-existing socket"
vm-run-until "kubectl describe pods/pod0 | grep '$errmsg_not_enough_exclcores'" ||
    error "cannot find expected error message from pod description"
out "Failed as expected"

kubectl delete pods --all --now || error "failed to delete pods"

out ""
out "### Request 0 cores from exclusive pool"
( CPU=1000m STP_SOCKET_ID=0 EXCLCORES=0 wait_t=5s create cmk-exclusive )  &&
    error "expected timeout, but pod launched with 0 cores from the exclusive pool"
vm-run-until "kubectl describe pods/pod0 | grep '$errmsg_zero_cores'" ||
    error "cannot find expected error message from pod description"
out "Failed as expected"

kubectl delete pods --all --now || error "failed to delete pods"

out ""
out "### Request more cores from socket 0 than available"
( CPU=3000m STP_SOCKET_ID=0 EXCLCORES=3 wait_t=5s create cmk-exclusive ) &&
    error "expected timeout, but pod got too many cores successfully"
vm-run-until "kubectl describe pods/pod0 | grep '$errmsg_not_enough_exclcores'" ||
    error "cannot find expected error message from pod description"
out "Failed as expected"

kubectl delete pods --all --now || error "failed to delete pods"

out ""
out "### Request more cores from socket 1 than available"
( CPU=1000m STP_SOCKET_ID=1 EXCLCORES=2 wait_t=5s create cmk-exclusive ) &&
    error "expected timeout, but pod got too many cores successfully"
vm-run-until "kubectl describe pods/pod0 | grep '$errmsg_not_enough_exclcores'" ||
    error "cannot find expected error message from pod description"
out "Failed as expected"

kubectl delete pods --all --now || error "failed to delete pods"
