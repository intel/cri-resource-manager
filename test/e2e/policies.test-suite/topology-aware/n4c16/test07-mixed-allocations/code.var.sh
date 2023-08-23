# Place pod0c0 and pod0c1 to shared pools on separate nodes.
CONTCOUNT=2 CPU=500m create guaranteed
report allowed
verify "len(mems['pod0c0']) == 1" \
       "len(mems['pod0c1']) == 1" \
       "disjoint_sets(mems['pod0c0'], mems['pod0c1'])" \
       "len(cpus['pod0c0']) == 4" \
       "len(cpus['pod0c1']) == 4" \
       "disjoint_sets(cpus['pod0c0'], cpus['pod0c1'])"

# Place pod1c0 to its own node, as there is still one 4-CPU node free.
# The placement of pod1c1 is more interesting:
# - node0 has only 3 CPUs (CPU #0 is reserved)
# - node1, node2 and node3 have containers in their shared pools
# - shared pools with pod0c* containers have more free space than node0
#   => pod1c0 should be place to either of those
# - because pod1c1 should get one exclusive CPU, either of pod0c0 and
#   pod0c1 should run in a shared pool of only 3 CPUs from now on.
CONTCOUNT=2 CPU=1500m create guaranteed
report allowed
verify `# every container is placed on a single node (no socket, no root)` \
       "[len(mems[c]) for c in mems] == [1] * len(mems)" \
       `# pod1c0 and pod1c1 are on different nodes` \
       "disjoint_sets(mems['pod1c0'], mems['pod1c1'])" \
       `# either of pod0c0 and pod0c1 has only 3 CPUs, the other has 4.` \
       "len(cpus['pod0c0']) == 3 or len(cpus['pod0c1']) == 3" \
       "len(cpus['pod0c0']) == 4 or len(cpus['pod0c1']) == 4" \
       `# pod1c0 and pod1c1 are allowed to use all CPUs on their nodes` \
       "len(cpus['pod1c0']) == 4" \
       "len(cpus['pod1c1']) == 4" \
       `# pod1c1 should have one exclusive CPU on its node` \
       "len(cpus['pod1c1'] - cpus['pod0c0'] - cpus['pod0c1']) == 1"

# Place pod2c0 to node0, as it has largest free shared pool (3 CPUs).
# Place pod2c1 to the node that has only either pod0c0 or pod0c1,
# while the other one of them already shares a node with pod1c1.
CONTCOUNT=2 CPU=2400m create guaranteed
report allowed
verify `# every container is placed on a single node (no socket, no root)` \
       "[len(mems[c]) for c in mems] == [1] * len(mems)" \
       `# pod1c1 should have kept its own exclusive CPU` \
       "len(cpus['pod1c1'] - set.union(*[cpus[c] for c in cpus if c != 'pod1c1'])) == 1" \
       `# pod2c0 is the only container in node0, so it happens to have 3 unshared CPUs for now` \
       "len(cpus['pod2c0']) == 3" \
       "len(cpus['pod2c0'] - set.union(*[cpus[c] for c in cpus if c != 'pod2c0'])) == 3" \
       `# pod2c1 shares its node and should not have exclusive CPUs` \
       "len(cpus['pod2c1']) == 4" \
       "len(cpus['pod2c1'] - set.union(*[cpus[c] for c in cpus if c != 'pod2c1'])) == 0" \
       `# pod2c1 should run in the same node as either pod0c0 or pod0c1` \
       "mems['pod2c1'] == mems['pod0c0'] or mems['pod2c1'] == mems['pod0c1']"

# pod3c0 should get 2 exclusive CPUs and 400m share from a shared pool.
# To get that, annotate the pod to:
# - opt-out from shared CPUs (=> opt-in to exclusive CPUs)
# - opt-in to isolated CPUs (this should not matter, test opt-out with pod4).
# There is only one node where the container fits: the same node as pod1c0.
ANNOTATIONS=('prefer-shared-cpus.cri-resource-manager.intel.com/pod: "false"'
             'prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"')
CONTCOUNT=1 CPU=2400m create guaranteed-annotated
report allowed
verify `# every container is placed on a single node (no socket, no root)` \
       "[len(mems[c]) for c in mems] == [1] * len(mems)" \
       `# pod3c0 and pod1c0 are placed in the same node` \
       "mems['pod3c0'] == mems['pod1c0']" \
       `# pod1c0 has 1 an exclusive CPU` \
       "len(cpus['pod1c0'] - set.union(*[cpus[c] for c in cpus if c != 'pod1c0'])) == 1" \
       `# pod3c0 has 2 exclusive CPUs` \
       "len(cpus['pod3c0'] - set.union(*[cpus[c] for c in cpus if c != 'pod3c0'])) == 2"

# Replace pod3 with pod4.
# Test release/(re)allocate mixed pod with exclusive CPUs and
# no-effect from isolated preference.
# - opt-out from shared CPUs (=> opt-in to exclusive CPUs)
# - opt-out from isolated CPUs (this does not affect getting exclusive CPUs)
kubectl delete pods pod3 --now --wait --ignore-not-found
ANNOTATIONS=('prefer-shared-cpus.cri-resource-manager.intel.com/pod: "false"'
             'prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "false"')
CONTCOUNT=1 CPU=2400m create guaranteed-annotated
report allowed
verify `# every container is placed on a single node (no socket, no root)` \
       "[len(mems[c]) for c in mems] == [1] * len(mems)" \
       `# pod4c0 and pod1c0 are placed in the same node` \
       "mems['pod4c0'] == mems['pod1c0']" \
       `# pod1c0 has 1 exclusive CPU` \
       "len(cpus['pod1c0'] - set.union(*[cpus[c] for c in cpus if c != 'pod1c0'])) == 1" \
       `# pod4c0 has 2 exclusive CPUs` \
       "len(cpus['pod4c0'] - set.union(*[cpus[c] for c in cpus if c != 'pod4c0'])) == 2"

# Replace pod1 with pod5.
# pod1 implicitly opted-in to exlusive CPUs due to 1500 mCPU request.
# Now explicitly opt-out of it by opting-in to shared-cpus.
kubectl delete pods pod1 --now --wait --ignore-not-found
# Make sure that shared pool size increased correctly after mixed pod deletion.
verify `# pod0c0 or pod0c1 shared a node with pod1c1 and had only 3 CPUs` \
       "len(cpus['pod0c0']) == 4" \
       "len(cpus['pod0c1']) == 4"

ANNOTATIONS=('prefer-shared-cpus.cri-resource-manager.intel.com/pod: "true"')
CONTCOUNT=2 CPU=1500m create guaranteed-annotated
report allowed
verify `# every container is placed on a single node (no socket, no root)` \
       "[len(mems[c]) for c in mems] == [1] * len(mems)" \
       `# pod5c0 should share a node with pod0c0 or pod0c1 and have access to all CPUs` \
       "mems['pod5c0'] == mems['pod0c0'] or mems['pod5c0'] == mems['pod0c1']" \
       "len(cpus['pod5c0']) == 4" \
       "len(cpus['pod0c0']) == 4" \
       "len(cpus['pod0c1']) == 4" \
       `# pod5c1 should run in a node with pod4c0 (this is where pod1c0 used to be)` \
       "mems['pod5c1'] == mems['pod4c0']" \
       "len(cpus['pod5c1']) == 2" \
       `# pod5c0 and pod5c1 share a node with another container => all their CPUs should be shared` \
       "len(cpus['pod5c0'] - set.union(*[cpus[c] for c in cpus if c != 'pod5c0'])) == 0" \
       "len(cpus['pod5c1'] - set.union(*[cpus[c] for c in cpus if c != 'pod5c1'])) == 0"
