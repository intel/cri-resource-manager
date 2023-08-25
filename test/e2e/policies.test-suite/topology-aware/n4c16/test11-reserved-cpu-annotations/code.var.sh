# Test that
# - containers marked in Annotations pinned on Reserved CPUs.

cri_resmgr_cfg_orig=$cri_resmgr_cfg

cleanup-test-pods() {
    ( kubectl delete pods pod0 --now --wait --ignore-not-found ) || true
    ( kubectl delete pods pod1 --now --wait --ignore-not-found ) || true
}
cleanup-test-pods

cri_resmgr_cfg_orig=$cri_resmgr_cfg
terminate cri-resmgr

AVAILABLE_CPU="cpuset:8-11"
RESERVED_CPU="cpuset:10-11"
cri_resmgr_cfg=$(instantiate cri-resmgr-reserved-annotations.cfg)
launch cri-resmgr

ANNOTATIONS='prefer-reserved-cpus.cri-resource-manager.intel.com/pod: "true"'
CONTCOUNT=1 create reserved-annotated
report allowed

ANNOTATIONS='prefer-reserved-cpus.cri-resource-manager.intel.com/container.special: "false"'
CONTCOUNT=1 create reserved-annotated
report allowed

verify 'cpus["pod0c0"] == {"cpu10", "cpu11"}'
verify 'cpus["pod1c0"] == {"cpu08"}'

cleanup-test-pods

terminate cri-resmgr
cri_resmgr_cfg=$cri_resmgr_cfg_orig
launch cri-resmgr
