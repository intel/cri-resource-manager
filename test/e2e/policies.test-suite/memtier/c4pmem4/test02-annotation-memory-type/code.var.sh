# Test that container memory is pinned according to memory-type annotation

# pod0c0 runs on the first node, uses only dram
# pod0c1 runs on the second node, uses only pmem
# pod0c2 runs on the third node, uses dram+pmem
# pod0c9 runs on the fourth node, no memory-type restrictions (use both)
MEM=250M MEMTYPEC0=dram MEMTYPEC1=pmem MEMTYPEC2=pmem,dram create memtype-guaranteed

verify 'cpus["pod0c0"] == {"cpu0"}' \
       'mems["pod0c0"] == {"node0"}' \
       'cpus["pod0c1"] == {"cpu1"}' \
       'mems["pod0c1"] == {"node7"}' \
       'cpus["pod0c2"] == {"cpu2"}' \
       'mems["pod0c2"] == {"node2", "node4"}' \
       'cpus["pod0c9"] == {"cpu3"}' \
       'mems["pod0c9"] == {"node3", "node5"}'
