terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/dyp-reserved.cfg launch cri-resmgr

cleanup() {
    vm-command \
        "kubectl delete pod -n kube-system --now pod0
         kubectl delete pod -n monitor-mypods --now pod1
         kubectl delete pod -n system-logs --now pod2
         kubectl delete pod -n kube-system --now pod3
         kubectl delete pods --now pod4 pod5 pod6
         kubectl delete pod -n kube-system --now pod7
         kubectl delete namespace monitor-mypods
         kubectl delete namespace system-logs
         kubectl delete namespace my-exact-name"
    return 0
}

cleanup

kubectl create namespace monitor-mypods
kubectl create namespace system-logs
kubectl create namespace my-exact-name

# pod0: kube-system
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
namespace=kube-system create dyp-busybox
report allowed
verify 'cpus["pod0c0"] == {"cpu00", "cpu01", "cpu02"}'

# pod1: match first ReservedPoolNamespaces glob, multicontainer
CPUREQ="1" MEMREQ="" CPULIM="1" MEMLIM=""
namespace=monitor-mypods CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod1c0"] == cpus["pod0c0"]' \
       'cpus["pod1c1"] == cpus["pod0c0"]'

# pod2: match last ReservedPoolNamespaces glob, slightly overbook reserved CPU
CPUREQ="1" MEMREQ="" CPULIM="1" MEMLIM=""
namespace=system-logs create dyp-busybox
report allowed
verify 'cpus["pod2c0"] == cpus["pod0c0"]'

# pod3: force a kube-system pod to full-core dynamic pool using an annotation
CPUREQ="2" MEMREQ="" CPULIM="2" MEMLIM=""
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: full-core" namespace=kube-system create dyp-busybox
report allowed
verify 'len(cpus["pod3c0"]) >= 2' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod3c0"])'

# pod4: run in shared dynamic pool
CPUREQ="2500m" MEMREQ="" CPULIM="2500m" MEMLIM=""
create dyp-busybox
report allowed
verify 'len(cpus["pod4c0"]) >= 3' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod3c0"], cpus["pod4c0"])'

# pod5: annotate otherwise a default pod to the reserved CPUs,
# severely overbook reserved CPUs
CPUREQ="2500m" MEMREQ="" CPULIM="2500m" MEMLIM=""
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: reserved" create dyp-busybox
report allowed
verify 'cpus["pod5c0"] == {"cpu00", "cpu01", "cpu02"}' \
       'disjoint_sets(cpus["pod5c0"], cpus["pod3c0"], cpus["pod4c0"])'

cleanup

# Now that all pods are deleted, make sure that cpus of reserved and
# default dynamic pools are as expected.

# pod6: run in shared dynamic pool
CPUREQ="999m" MEMREQ="" CPULIM="999m" MEMLIM=""
create dyp-busybox
report allowed
verify 'len(cpus["pod6c0"]) >= 1'

# pod7: kube-system
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
namespace=kube-system create dyp-busybox
report allowed
verify 'cpus["pod7c0"] == {"cpu00", "cpu01", "cpu02"}'

cleanup

terminate cri-resmgr
launch cri-resmgr
