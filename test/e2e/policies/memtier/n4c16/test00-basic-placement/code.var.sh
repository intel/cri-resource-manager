
# pod0: Test that 4 guaranteed containers eligible for isolated CPU allocation
# gets evenly spread over NUMA nodes.
CONTCOUNT=4 CPU=1 create guaranteed
report allowed
verify \
    'len(cpus["pod0c0"]) == 1' \
    'len(cpus["pod0c1"]) == 1' \
    'len(cpus["pod0c2"]) == 1' \
    'len(cpus["pod0c3"]) == 1' \
    'disjoint_sets(cpus["pod0c0"], cpus["pod0c1"], cpus["pod0c2"], cpus["pod0c3"])' \
    'disjoint_sets(nodes["pod0c0"], nodes["pod0c1"], nodes["pod0c2"], nodes["pod0c3"])'

kubectl delete pods --all --now

# pod1: Test that 4 guaranteed containers not eligible for isolated CPU allocation
# gets evenly spread over NUMA nodes.
CONTCOUNT=4 CPU=3 create guaranteed
report allowed
verify \
    'len(cpus["pod1c0"]) == 3' \
    'len(cpus["pod1c1"]) == 3' \
    'len(cpus["pod1c2"]) == 3' \
    'len(cpus["pod1c3"]) == 3' \
    'disjoint_sets(cpus["pod1c0"], cpus["pod1c1"], cpus["pod1c2"], cpus["pod1c3"])' \
    'disjoint_sets(nodes["pod1c0"], nodes["pod1c1"], nodes["pod1c2"], nodes["pod1c3"])'

kubectl delete pods --all --now

# pod2: Test that 4 burstable containers not eligible for isolated/exclusive CPU allocation
# gets evenly spread over NUMA nodes.
CONTCOUNT=4 CPUREQ=3 CPULIM=4 create burstable
report allowed
verify \
    'disjoint_sets(cpus["pod2c0"], cpus["pod2c1"], cpus["pod2c2"], cpus["pod2c3"])' \
    'disjoint_sets(nodes["pod2c0"], nodes["pod2c1"], nodes["pod2c2"], nodes["pod2c3"])'

kubectl delete pods --all --now
