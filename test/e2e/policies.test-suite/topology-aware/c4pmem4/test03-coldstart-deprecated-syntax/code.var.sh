# Test that a cold-started pod...
# 1. is allowed to allocate memory only from PMEM nodes
#    during cold period (of length $DURATION_S).
# 2. is restricted from the very beginning of pod execution:
#    immediately allocated memory blob consumes PMEM from expected node.
# 3. is allowed to allocate memory from both PMEM and DRAM after
#    the cold period.
# 4. is no more restricted after $DURATION_S + 1s has passed in pod:
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

DURATION_S=10
COLD_ALLOC_KB=$((50 * 1024))
WARM_ALLOC_KB=$((100 * 1024))
MEM=1G
create bb-coldstart

echo "Wait that coldstart period is started for the pod"
vm-run-until "$CRI_RESMGR_OUTPUT | grep 'coldstart: triggering coldstart for pod0:pod0c0'" ||
    error "cri-resmgr did not report triggering coldstart period"

verify 'cores["pod0c0"] == {"node1/core0"}' \
       "mems['pod0c0'] == {'node7'}"

echo "Wait that the pod has finished memory allocation during cold period."
vm-run-until "pgrep -f '^sh -c paused after cold_alloc'" >/dev/null ||
    error "cold memory allocation timed out"

echo "Verify PMEM consumption during cold period."
# meminfo MemUsed vs dd bytes error margin, use 10%
PMEM_ERROR_MARGIN=$((COLD_ALLOC_KB / 10))
sleep 1
PMEM_USED_COLD_POD0="$(pmem-used)"
PMEM_COLD_CONSUMED=$(( $PMEM_USED_COLD_POD0 - $PMEM_USED_BEFORE_POD0 ))
if (( $PMEM_COLD_CONSUMED + $PMEM_ERROR_MARGIN < $COLD_ALLOC_KB )); then
    error "pod0 did not allocate ${COLD_ALLOC_KB}kB from PMEM. MemUsed PMEM delta: $PMEM_COLD_CONSUMED"
else
    echo "### Verified: PMEM memory consumed during cold period: $PMEM_COLD_CONSUMED kB, pod script allocated: $COLD_ALLOC_KB kB"
fi

coldstarts=$(vm-command-q "$CRI_RESMGR_OUTPUT | grep 'finishing coldstart period for pod0:pod0c0' | wc -l")
echo "Wait that cri-resmgr finishes coldstart period within $(($DURATION_S + 10)) seconds."
vm-run-until --timeout $((DURATION_S + 10)) "[ \$($CRI_RESMGR_OUTPUT | grep 'finishing coldstart period for pod0:pod0c0' | wc -l) -gt $coldstarts ]" ||
    error "cri-resmgr did not report finishing coldstart period within $DURATION_S seconds"

vm-command "$CRI_RESMGR_OUTPUT | grep 'pinning to memory 1,7'" ||
    error "cri-resmgr did not report pinning to expected memory nodes"

verify 'cores["pod0c0"] == {"node1/core0"}' \
       'mems["pod0c0"] == {"node1", "node7"}'

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
    echo "### Verify (soft) failed: pod0 allocated $WARM_ALLOC_KB kB from PMEM. Should have been taken from DRAM."
else
    echo "### Verified (soft): PMEM memory consumption delta during warm period: $PMEM_WARM_CONSUMED kB, pod script allocated: $WARM_ALLOC_KB kB"
fi
