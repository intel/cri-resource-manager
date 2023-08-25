# Test placing containers with and without annotations to correct pools
# reserved and shared CPUs.

( kubectl delete pods pod3 -n kube-system --now --wait --ignore-not-found ) || true

# pod0: singlecpu
out ""
out "### Multicontainer pod, all containers run on single CPU"
# singlecpu pool has capacity for two pods => 500 mCPU/pod
# test with 3 containers per pod => 167 mCPU/container
CPUREQ="167m" MEMREQ="" CPULIM="" MEMLIM=""
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: singlecpu" CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod0c1"] == cpus["pod0c2"]' \
       'cpus["pod0c0"] == expected.cpus.singlecpu[0]' \
       'mems["pod0c0"] == expected.mems.singlecpu[0]'

# pod1: dualcpu
out ""
out "### Multicontainer pod, all containers run on two CPUs."
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod1c0"] == cpus["pod1c1"] == cpus["pod1c2"]' \
       'cpus["pod1c0"] == expected.cpus.dualcpu[0]' \
       'mems["pod1c1"] == expected.mems.dualcpu[0]'

# pod2: default
out ""
out "### Multicontainer pod, no annotations. Runs on shared CPUs."
CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod2c0"] == cpus["pod2c1"] == cpus["pod2c2"]' \
       'cpus["pod2c0"] == expected.cpus.default[0]' \
       'mems["pod2c2"] == expected.mems.default[0]'

# pod3: reserved
out ""
out "### Multicontainer pod in kube-system namespace. Runs on reserved CPUs."
namespace=kube-system CONTCOUNT=3 create podpools-busybox
report allowed
verify 'cpus["pod3c0"] == cpus["pod3c1"] == cpus["pod3c2"]' \
       'cpus["pod3c0"] == expected.cpus.reserved[0]' \
       'mems["pod3c0"] == expected.mems.reserved[0]'

kubectl delete pods pod3 -n kube-system --now --wait --ignore-not-found

# pod4: bad pool name
out ""
out "### Single container pod, fallback to the default pool."
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: non-existing-pool" create podpools-busybox
report allowed
verify 'cpus["pod4c0"] == expected.cpus.default[0]' \
       'mems["pod4c0"] == expected.mems.default[0]'

kubectl delete pods pod0 pod1 pod2 --now --wait --ignore-not-found
