# Test that CPU-less PMEM nodes are assigned to closest nodes with CPU.

# Restart cri-resmgr in order to clear logs and make sure assignment
# is successful with installed cri-resmgr.
terminate cri-resmgr
launch cri-resmgr

CRI_RESMGR_OUTPUT_COMMAND="cat cri-resmgr.output.txt"

echo "Verify PMEM node assignment to CPU-ful nodes"
for expected_output in \
    "PMEM node #4 assigned to .*#2" \
    "PMEM node #5 assigned to .*#3" \
    "PMEM node #6 assigned to .*#0" \
    "PMEM node #7 assigned to .*#1"; do
    vm-command "$CRI_RESMGR_OUTPUT_COMMAND | grep -E '$expected_output'" ||
        command-error "expected PMEM assignment not found"
done
