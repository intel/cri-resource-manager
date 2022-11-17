# Test that container memory is pinned according to memory-type annotation

# pod0c0 runs on node 1, uses only dram
# pod0c1 runs on node 2, uses only pmem
# pod0c2 runs on node 3, uses dram+pmem
# pod0c9 runs on root node (all non-reserved CPUs),
#     no memory-type restrictions (=> use all memory nodes)
MEM=250M MEMTYPEC0=dram MEMTYPEC1=pmem MEMTYPEC2=pmem,dram create memtype-guaranteed
report allowed

if [ "$VM_CRI_DS" == "1" ]; then
    # Slightly different node allocation when using DaemonSet
    verify 'cpus["pod0c0"] == {"cpu2"}' \
	   'mems["pod0c0"] == {"node2"}' \
	   'cpus["pod0c1"] == {"cpu3"}' \
	   'mems["pod0c1"] == {"node5"}' \
	   'cpus["pod0c2"] == {"cpu2"}' \
	   'mems["pod0c2"] == {"node2", "node4"}' \
	   'cpus["pod0c9"] == {"cpu3"}' \
	   'mems["pod0c9"] == {"node3", "node5"}'
else
    verify 'cpus["pod0c0"] == {"cpu1"}' \
	   'mems["pod0c0"] == {"node1"}' \
	   'cpus["pod0c1"] == {"cpu2"}' \
	   'mems["pod0c1"] == {"node4"}' \
	   'cpus["pod0c2"] == {"cpu3"}' \
	   'mems["pod0c2"] == {"node3", "node5"}' \
	   'cpus["pod0c9"] == {"cpu1", "cpu2", "cpu3"}' \
	   'mems["pod0c9"] == {"node0", "node1", "node2", "node3", "node4", "node5", "node6", "node7"}'
fi
