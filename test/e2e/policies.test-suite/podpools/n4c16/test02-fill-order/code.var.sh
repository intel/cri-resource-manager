# Test filling pools with pods in correct order

# Test only BestEffort containers
CPUREQ="" MEMREQ="" CPULIM="" MEMLIM=""

# pod0..2: balanced filling, every singlecpu pool should have one pod
out "### Filling singlecpu pool in Balanced fill order"
n=3 POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: singlecpu" CONTCOUNT=2 create podpools-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod0c1"]' \
       'cpus["pod1c0"] == cpus["pod1c1"]' \
       'cpus["pod2c0"] == cpus["pod2c1"]' \
       'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod1c0"]) == 1' \
       'len(cpus["pod2c0"]) == 1' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"])'

# pod3..5: balanced filling up to max, every singlecpu pool should have two pods
n=3 POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: singlecpu" CONTCOUNT=2 create podpools-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod0c1"]' \
       'cpus["pod1c0"] == cpus["pod1c1"]' \
       'cpus["pod2c0"] == cpus["pod2c1"]' \
       'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod1c0"]) == 1' \
       'len(cpus["pod2c0"]) == 1' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"])' \
       'cpus["pod3c0"] == cpus["pod3c1"]' \
       'cpus["pod4c0"] == cpus["pod4c1"]' \
       'cpus["pod5c0"] == cpus["pod5c1"]' \
       'len(cpus["pod3c0"]) == 1' \
       'len(cpus["pod4c0"]) == 1' \
       'len(cpus["pod5c0"]) == 1' \
       'disjoint_sets(cpus["pod3c0"], cpus["pod4c0"], cpus["pod5c0"])' \
       'cpus["pod5c0"] == cpus["pod2c0"]' # the last pool should have been filled by pods 2 and 5

# make a little room to the first pool and clear the last pool
kubectl delete pods pod0 pod2 pod5 --now

# pod6: Balanced fill order should place this pod to the last pool (it has maximal free space)
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: singlecpu" CONTCOUNT=1 create podpools-busybox
report allowed
verify 'disjoint_sets(cpus["pod6c0"],
                      set.union(cpus["pod1c0"], cpus["pod3c0"], cpus["pod4c0"]))'

kubectl delete pods --all --now
reset counters

out "### Filling dualcpu pool in Packed fill order"
# pod0..2: should go to the first pool
n=3 POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CONTCOUNT=1 create podpools-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod1c0"] == cpus["pod2c0"]'

# pod3..5: should go to the second pool
n=3 POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CONTCOUNT=1 create podpools-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod1c0"] == cpus["pod2c0"]' \
       'cpus["pod3c0"] == cpus["pod4c0"] == cpus["pod5c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod3c0"])'

# Deleting two pods from the first pool, one from the last.
kubectl delete pods pod0 pod1 pod5

# pod6: Packed fill order should place this to the last pool (it has minimal free space)
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CONTCOUNT=1 create podpools-busybox
report allowed
verify 'cpus["pod3c0"] == cpus["pod4c0"] == cpus["pod6c0"]' \
       'disjoint_sets(cpus["pod2c0"], cpus["pod6c0"])'
