# pod0: require 10 out of 16 CPUs with two containers.
# Both containers should fit in their own die. (8 CPUs per die.)
CPU=5 CONTCOUNT=2 create guaranteed
report allowed
verify \
    'len(cpus["pod0c0"]) == 5' \
    'len(cpus["pod0c1"]) == 5' \
    'len(nodes["pod0c0"]) == len(nodes["pod0c1"]) == 2' \
    'len(dies["pod0c0"]) == len(dies["pod0c1"]) == 1' \
    'disjoint_sets(cpus["pod0c0"], cpus["pod0c1"])'

# pod1: two containers in a besteffort pod.
CONTCOUNT=2 create besteffort
report allowed
verify \
    'len(cpus["pod0c0"]) == 5' \
    'len(cpus["pod0c1"]) == 5' \
    'disjoint_sets(set.union(cpus["pod0c0"], cpus["pod0c1"]))' \
    'len(cpus["pod1c0"]) > 0' \
    'len(cpus["pod1c1"]) > 0' \
    'disjoint_sets(
         set.union(cpus["pod0c0"], cpus["pod0c1"]),
         set.union(cpus["pod1c0"], cpus["pod1c1"]))'

# Delete pod0
delete pods/pod0 --now
report allowed

# Next squeeze the besteffort containers to the minimum.

# pod2: 4 guaranteed containers, each requiring 3 CPUs.
CPU=3 CONTCOUNT=4 create guaranteed
report allowed
verify \
    'len(cpus["pod2c0"]) == len(cpus["pod2c1"]) == len(cpus["pod2c2"]) == len(cpus["pod2c3"]) == 3' \
    'disjoint_sets(cpus["pod2c0"], cpus["pod2c1"], cpus["pod2c2"], cpus["pod2c3"])'

# pod3: 1 guaranteed container taking the last non-reserved CPU
# that can be taken from shared pools.
CPU=1 create guaranteed
report allowed
verify \
    'disjoint_sets(
         set.union(cpus["pod1c0"], cpus["pod1c1"]),
         set.union(cpus["pod3c0"],
                   cpus["pod2c0"], cpus["pod2c1"], cpus["pod2c2"], cpus["pod2c3"]))'
