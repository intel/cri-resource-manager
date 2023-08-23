# Test that
# - containers marked in ReservedPoolNamespaces option pinned on Reserved CPUs.

(kubectl create namespace reserved-test) || true

cri_resmgr_cfg_orig=$cri_resmgr_cfg

# This script will create pods to the reserved and default namespace.
# Make sure the namespace is clear when starting the test and clean it up
# if exiting with success. Otherwise leave the pod running for
# debugging in case of a failure.
cleanup-test-pods() {
    ( kubectl delete pods pod0 -n kube-system --now --wait --ignore-not-found ) || true
    ( kubectl delete pods pod1 --now --wait --ignore-not-found ) || true
}
cleanup-test-pods

terminate cri-resmgr
AVAILABLE_CPU="cpuset:8-11"
RESERVED_CPU="cpuset:10-11"
cri_resmgr_cfg=$(instantiate cri-resmgr-reserved-namespaces.cfg)
launch cri-resmgr

CONTCOUNT=1 namespace=kube-system create besteffort
CONTCOUNT=1 create besteffort
report allowed
verify 'cpus["pod0c0"] == {"cpu10", "cpu11"}'
verify 'cpus["pod1c0"] == {"cpu08", "cpu09"}'

cleanup-test-pods

# Test that
# - containers that are namespace-assigned to reserved pools are pinned there
# - containers that are annotated to opt-put that are pinned elsewhere, and
# - containers that are namespace-assigned and annotated to reserved pools are pinned there

(kubectl create namespace foobar) || true

cleanup-foobar-namespace() {
    (kubectl delete pods -n foobar --all --now --wait) || true
}
cleanup-foobar-namespace

CONTCOUNT=1 namespace=foobar create besteffort
ANN0='prefer-reserved-cpus.cri-resource-manager.intel.com/pod: "false"'
CONTCOUNT=1 namespace=foobar create besteffort
ANN0='prefer-reserved-cpus.cri-resource-manager.intel.com/pod: "true"'
CONTCOUNT=1 namespace=foobar create besteffort

report allowed
verify 'cpus["pod2c0"] == {"cpu10", "cpu11"}'
verify 'cpus["pod3c0"] == {"cpu08", "cpu09"}'
verify 'cpus["pod4c0"] == {"cpu10", "cpu11"}'

cleanup-foobar-namespace
