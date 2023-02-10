# Test placing containers with and without annotations to correct dynamic pools
# reserved and shared CPUs.

cleanup() {
    vm-command "kubectl delete pods pod0 -n kube-system; kubectl delete pods -n pool1 --all --now; kubectl delete pods --all --now; kubectl delete namespace pool1"
    return 0
}

cleanup

terminate cri-resmgr

launch cri-resmgr

# pod0: run on reserved CPUs.
namespace=kube-system CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod0c1"]' \
       'len(cpus["pod0c0"]) == 1'

# pod1: run in shared dynamic pool.
# We do not add annotations to this pod, and we do not set any
# namespace, so this pod is expected to be created to the shared pool.
create dyp-busybox
report allowed
verify 'len(cpus["pod1c0"]) == 14'

# The size of each dynamic pool is obtained by adding the requests of the containers in this pool and the CPUs allocated based on cpu utilization,
# so the size of each dynamic pool is greater than or equal to the sum of the requests of the containers in the pool.

# pod2: run in the pool1.
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: pool1" CONTCOUNT=1 create dyp-busybox
report allowed
verify 'len(cpus["pod2c0"]) >= 1' \
      'len(cpus["pod1c0"]) + len(cpus["pod2c0"]) == 14' \
      'disjoint_sets(cpus["pod2c0"], cpus["pod1c0"])'

# pod3: run in the pool1.
CPUREQ="1500m" MEMREQ="100M" CPULIM="1500m" MEMLIM="100M"
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: pool1" CONTCOUNT=1 create dyp-busybox
report allowed
verify 'cpus["pod2c0"] == cpus["pod3c0"]' \
      'len(cpus["pod3c0"]) >= 2' \
      'len(cpus["pod1c0"]) + len(cpus["pod3c0"]) == 14' \
      'disjoint_sets(cpus["pod1c0"], cpus["pod3c0"])'

# pod4: run in the pool2.
CPUREQ="1500m" MEMREQ="100M" CPULIM="1500m" MEMLIM="100M"
POD_ANNOTATION="dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: pool2" CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod4c0"] == cpus["pod4c1"]' \
      'len(cpus["pod4c0"]) >= 3' \
      'len(cpus["pod3c0"]) >= 2' \
      'len(cpus["pod1c0"]) + len(cpus["pod3c0"]) + len(cpus["pod4c0"]) == 14' \
      'disjoint_sets(cpus["pod4c0"], cpus["pod3c0"], cpus["pod1c0"])'

# pod5: run in the pool1.
CPUREQ="1500m" MEMREQ="100M" CPULIM="1500m" MEMLIM="100M"
kubectl create namespace "pool1"
namespace="pool1" CONTCOUNT=1 create dyp-busybox
report allowed
verify 'cpus["pod5c0"] == cpus["pod2c0"]'\
      'len(cpus["pod5c0"]) >= 4' \
      'len(cpus["pod4c0"]) >= 3' \
      'len(cpus["pod1c0"]) + len(cpus["pod3c0"]) + len(cpus["pod4c0"]) == 14'

cleanup

