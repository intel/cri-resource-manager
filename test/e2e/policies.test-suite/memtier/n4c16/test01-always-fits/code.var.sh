# Test that guaranteed and burstable pods get the CPUs they require
# when there are enough CPUs available.

# pod0, fits in a core
CPU=1 create guaranteed
report allowed
verify \
    'node_ids(nodes["pod0c0"]) == {1}' \
    'cpu_ids(cpus["pod0c0"]) == {4}'

# pod1, takes full core - from a different node than pod0
CPU=2 create guaranteed
report allowed
verify \
    'cpu_ids(cpus["pod0c0"]) == {4}' \
    'node_ids(nodes["pod1c0"]) == {2}' \
    'cpu_ids(cpus["pod1c0"]) == {8, 9}'

# pod2, does not fit in a core but fits in a node
CPU=3 create guaranteed
report allowed
verify \
    'len(cpus["pod0c0"]) == 1' \
    'len(cpus["pod1c0"]) == 2' \
    'len(cores["pod1c0"]) == 1' \
    'len(cpus["pod2c0"]) == 3' \
    'len(cores["pod2c0"]) == 2' \
    'len(nodes["pod2c0"]) == 1' \
    'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"])'

# pod3, tries to fully exhaust the shared subset of a (NUMA node) pool
# Currently memtier refuses to exhaust even idle shared CPU subsets of
# a pool. Therefore such attempts will try to squeeze the container to
# another pool at the same level or, if none found, push the container
# one level up to the parent pool.
#
# There is a pending commit to change this behavior to allow exhausting
# fully idle subsets (no active shared grants). Once that lands, update
# this test accordingly as well.
CPU=4 create guaranteed
report allowed
verify \
    'len(cpus["pod0c0"]) == 1' \
    'len(cpus["pod1c0"]) == 2' \
    'len(cores["pod1c0"]) == 1' \
    'len(cpus["pod2c0"]) == 3' \
    'len(cores["pod2c0"]) == 2' \
    'len(nodes["pod2c0"]) == 1' \
    'len(cpus["pod3c0"]) == 4' \
    'len(cores["pod3c0"]) == 2' \
    'len(nodes["pod3c0"]) == 2' \
    'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"], cpus["pod3c0"])'

kubectl delete pods --all --now

# pod4, fits in a die/package
CPU=5 create guaranteed
report allowed
verify \
    'len(cpus["pod4c0"]) == 5' \
    'len(cores["pod4c0"]) == 3' \
    'len(nodes["pod4c0"]) == 2' \
    'len(dies["pod4c0"]) == 1'

# pod5, takes a full die/package
# cpu0 is reserved, so allocating 7 CPUs is expected to fill package0/die0
CPU=7 create guaranteed
report allowed
verify \
    'len(cpus["pod4c0"]) == 5' \
    'len(cores["pod4c0"]) == 3' \
    'len(nodes["pod4c0"]) == 2' \
    'len(dies["pod4c0"]) == 1' \
    'len(cpus["pod5c0"]) == 7' \
    'len(cores["pod5c0"]) == 4' \
    'len(dies["pod5c0"]) == 1' \
    'disjoint_sets(cpus["pod4c0"], cpus["pod5c0"])'

kubectl delete pods --all --now

# pod6, doesn't fit in a die/package, needs virtual root
CPU=9 create guaranteed
report allowed
verify \
    'len(cpus["pod6c0"]) == 9' \
    'len(packages["pod6c0"]) == 2'

kubectl delete pods --all --now

reset counters

# pod0, burstable containers must get at least the cores they require
CPUREQ=3 CPULIM=$(( CPUREQ + 1 )) create burstable
report allowed
verify \
    'len(cpus["pod0c0"]) >= 2'

# pod1
CPUREQ=4 CPULIM=$(( CPUREQ + 1 )) create burstable
report allowed
verify \
    'len(cpus["pod0c0"]) >= 2' \
    'len(cpus["pod1c0"]) >= 4'

# pod2
CPUREQ=5 CPULIM=$(( CPUREQ + 1 )) create burstable
report allowed
verify \
    'len(cpus["pod0c0"]) >= 2' \
    'len(cpus["pod1c0"]) >= 4' \
    'len(cpus["pod2c0"]) >= 5'

kubectl delete pods pod0 pod1 --now

# pod3
CPUREQ=8 CPULIM=$(( CPUREQ + 1 )) create burstable
report allowed
verify \
    'len(cpus["pod2c0"]) >= 5' \
    'len(cpus["pod3c0"]) >= 8'

kubectl delete pods pod3 --now

# pod4, pod5 (and existing pod2) take 5 and 4 CPUs. As there are 8
# CPUs/node, pod2 and pod4 have consumed free node
# pairs/dies/packages. pod5 will be spread across nodes.
CPUREQ=5 CPULIM=$(( CPUREQ + 1 )) create burstable
report allowed
CPUREQ=4 CPULIM=$(( CPUREQ + 1 )) create burstable
report allowed
verify \
    'len(cpus["pod2c0"]) >= 5' \
    'len(cpus["pod4c0"]) >= 5' \
    'len(cpus["pod5c0"]) >= 4'
