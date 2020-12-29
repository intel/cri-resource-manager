# Test placing containers with and without annotations to correct pools
# reserved and shared CPUs.

( kubectl delete pods pod3 -n kube-system --now ) || true

# pod0: singlecpu
out ""
out "### Multicontainer pod, all containers run on single CPU"
# singlecpu pool has capacity for two pods => 500 mCPU/pod
# test with 3 containers per pod => 167 mCPU/container
CPUREQ="167m" MEMREQ="" CPULIM="" MEMLIM=""
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: singlecpu" CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod0c1"] == cpus["pod0c2"]' \
       'len(cpus["pod0c0"]) == 1' \
       'mems["pod0c0"] == {"node0"}'

# pod1: dualcpu
out ""
out "### Multicontainer pod, all containers run on two CPUs."
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod1c0"] == cpus["pod1c1"] == cpus["pod1c2"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"])' \
       'len(cpus["pod1c0"]) == 2' \
       'mems["pod1c1"] == {"node1"}'

# pod2: default
out ""
out "### Multicontainer pod, no annotations. Runs on shared CPUs."
CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod2c0"] == cpus["pod2c1"] == cpus["pod2c2"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"])' \
       'len(cpus["pod2c0"]) == 4' \
       'mems["pod2c2"] == {"node3", "node1"}'

# pod3: reserved
out ""
out "### Multicontainer pod in kube-system namespace. Runs on reserved CPUs."
namespace=kube-system CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod3c0"] == cpus["pod3c1"] == cpus["pod3c2"] == {"cpu15"}' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"], cpus["pod3c0"])' \
       'mems["pod3c0"] == {"node3"}'

kubectl delete pods pod3 -n kube-system --now

# pod4: bad pool name
out ""
out "### Single container pod, fallback to the default pool."
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: non-existing-pool" create podpools-busybox
report allowed
verify 'cpus["pod4c0"] == cpus["pod2c0"]' \
       'mems["pod4c0"] == mems["pod2c0"]'

kubectl delete pods pod0 pod1 pod2 --now
