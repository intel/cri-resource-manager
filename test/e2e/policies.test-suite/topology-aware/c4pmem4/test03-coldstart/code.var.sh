# Test that a cold-started pod...
# 1. is allowed to allocate memory only from PMEM nodes
#    during cold period (of length $DURATION).
# 2. is restricted from the very beginning of pod execution:
#    immediately allocated memory blob consumes PMEM from expected node.
# 3. is allowed to allocate memory from both PMEM and DRAM after
#    the cold period.
# 4. is no more restricted after $DURATION + 1s has passed in pod:
#    warm-allocated memory is not taken from PMEM nodes.

PMEM_NODES='{"node4", "node5", "node6", "node7"}'

# pmem-used returns total MemUsed (allocated) memory on PMEM nodes
pmem-used() {
    local pmem_nodes_shell=${PMEM_NODES//[\" ]/}
    vm-command "cat /sys/devices/system/node/$pmem_nodes_shell/meminfo | awk '/MemUsed:/{mem+=\$4}END{print mem}'" >/dev/null ||
        command-error "cannot read PMEM usage from node $node"
    echo "$COMMAND_OUTPUT"
}

CRI_RESMGR_OUTPUT="cat cri-resmgr.output.txt"

PMEM_USED_BEFORE_POD0="$(pmem-used)"

DURATION=10s
COLD_ALLOC=$((10 * 1024))kB
WARM_ALLOC=$((20 * 1024))kB
MEM=1G
create bb-coldstart

echo "Wait that coldstart period is started for the pod"
vm-run-until "$CRI_RESMGR_OUTPUT | grep 'coldstart: triggering coldstart for pod0:pod0c0'" ||
    error "cri-resmgr did not report triggering coldstart period"
report allowed

if [ "$VM_CRI_DS" == "1" ]; then
    # Slightly different node allocation when using DaemonSet
    verify 'cores["pod0c0"] == {"node2/core0"}' \
	   "mems['pod0c0'] == {'node4'}"
else
    verify 'cores["pod0c0"] == {"node1/core0"}' \
	   "mems['pod0c0'] == {'node7'}"
fi

echo "Wait that the pod has finished memory allocation during cold period."
vm-run-until "pgrep -f '^sh -c paused after cold_alloc'" >/dev/null ||
    error "cold memory allocation timed out"

echo "Verify PMEM consumption during cold period."
PMEM_ERROR_MARGIN=1024 # meminfo MemUsed vs dd bytes error margin
sleep 1
PMEM_USED_COLD_POD0="$(pmem-used)"
PMEM_COLD_CONSUMED=$(( $PMEM_USED_COLD_POD0 - $PMEM_USED_BEFORE_POD0 ))
if (( $PMEM_COLD_CONSUMED + $PMEM_ERROR_MARGIN < ${COLD_ALLOC%kB} )); then
    error "pod0 did not allocate $COLD_ALLOC from PMEM. MemUsed PMEM delta: $PMEM_COLD_CONSUMED"
else
    echo "### Verified: PMEM memory consumed during cold period: $PMEM_COLD_CONSUMED kB, pod script allocated: ${COLD_ALLOC%kB} kB"
fi

coldstarts=$(vm-command-q "$CRI_RESMGR_OUTPUT | grep 'finishing coldstart period for pod0:pod0c0' | wc -l")
echo "Wait that cri-resmgr finishes coldstart period within 5s + $DURATION."
sleep 5s
vm-run-until --timeout ${DURATION%s} "[ \$($CRI_RESMGR_OUTPUT | grep 'finishing coldstart period for pod0:pod0c0' | wc -l) -gt $coldstarts ]" ||
    error "cri-resmgr did not report finishing coldstart period within $DURATION"

report allowed

if [ "$VM_CRI_DS" == "1" ]; then
    # Slightly different node allocation when using DaemonSet
    vm-command "$CRI_RESMGR_OUTPUT | grep 'pinning to memory 2,4'" ||
	error "cri-resmgr did not report pinning to expected memory nodes"

    verify 'cores["pod0c0"] == {"node2/core0"}' \
       'mems["pod0c0"] == {"node4"}'
else
    vm-command "$CRI_RESMGR_OUTPUT | grep 'pinning to memory 1,7'" ||
	error "cri-resmgr did not report pinning to expected memory nodes"

    verify 'cores["pod0c0"] == {"node1/core0"}' \
       'mems["pod0c0"] == {"node1", "node7"}'
fi

echo "Let the pod continue from cold_alloc to warm_alloc."
vm-command 'kill -9 $(pgrep -f "^sh -c paused after cold_alloc")'

echo "Make sure that bb-coldstart finishes allocating memory in warm mode."
vm-run-until "pgrep -f '^sh -c paused after warm_alloc'"  ||
    error "warm memory allocation timed out"

echo "Verify (soft): PMEM consumption after cold period."
sleep 1
PMEM_USED_WARM_POD0="$(pmem-used)"
PMEM_WARM_CONSUMED=$(( $PMEM_USED_WARM_POD0 - $PMEM_USED_COLD_POD0 ))
if (( $PMEM_WARM_CONSUMED > 0 )); then
    echo "### Verify (soft) failed: pod0 allocated $WARM_ALLOC from PMEM. Should have been taken from DRAM."
else
    echo "### Verified (soft): PMEM memory consumption delta during warm period: $PMEM_WARM_CONSUMED kB, pod script allocated: ${WARM_ALLOC%kB} kB"
fi
