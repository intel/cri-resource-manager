# Test placing containers with and without annotations to correct balloons
# reserved and shared CPUs.

cleanup() {
    vm-command "kubectl delete pods -n kube-system pod0; kubectl delete pods --all --now --wait; kubectl delete namespace three --now --wait --ignore-not-found"
    return 0
}

cleanup

# pod0: run on reserved CPUs
namespace=kube-system CONTCOUNT=2 create balloons-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod0c1"]' \
       'len(cpus["pod0c0"]) == 1'

# pod1: run on the same two-cpu balloon (running containers of a pod
# on the same balloon takes precedence creating new balloons).
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: two-cpu" CONTCOUNT=2 create balloons-busybox
report allowed
verify 'cpus["pod1c0"] == cpus["pod1c1"]' \
       'len(cpus["pod1c0"]) == 2'

# pod2: run on a different two-cpu balloon than pod1 (new balloon
# creation is preferred).
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: two-cpu" CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod2c0"]) == 2' \
       'disjoint_sets(cpus["pod2c0"], cpus["pod1c0"])'

# pod3: fits exactly on a single three-cpu instance. No need to create
# new balloon even if spreading pods is preferred.
CPUREQ="1500m" MEMREQ="100M" CPULIM="1500m" MEMLIM="100M"
kubectl create namespace "three"
namespace="three" CONTCOUNT=2 create balloons-busybox
report allowed
verify 'cpus["pod3c0"] == cpus["pod3c1"]' \
       'len(cpus["pod3c0"]) == 3'

cleanup

# pod4: first two containers to the first instance, 3rd to new four-cpu instance
CPUREQ="3" MEMREQ="" CPULIM="3" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: four-cpu" CONTCOUNT=3 create balloons-busybox
report allowed
verify 'cpus["pod4c0"] == cpus["pod4c1"]' \
       'disjoint_sets(cpus["pod4c2"], cpus["pod4c0"])' \
       'len(cpus["pod4c0"]) == 6' \
       'len(cpus["pod4c2"]) == 4'

cleanup

# pod5: all spread containers to their own balloon instances
CPUREQ="1250m" MEMREQ="" CPULIM="" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: five-cpu" CONTCOUNT=3 create balloons-busybox
report allowed
verify 'disjoint_sets(cpus["pod5c0"], cpus["pod5c1"], cpus["pod5c2"])' \
       'len(cpus["pod5c0"]) == 2' \
       'len(cpus["pod5c1"]) == 2' \
       'len(cpus["pod5c2"]) == 2'

cleanup
