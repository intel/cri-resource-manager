CRI_RESMGR_OUTPUT="cat cri-resmgr.output.txt"

# pod0: 2 containers, both get their own socket and take > 50 % of their socket's mem
CPU=200m MEM=5G CONTCOUNT=2 create guaranteed
report allowed
verify 'len(mems["pod0c0"]) == 2' \
       'len(mems["pod0c1"]) == 2' \
       'mems["pod0c0"] != mems["pod0c1"]'

# pod1: reserve neglible amount of memory from the root node
# This must not cause raising pod0 containers raising upwards.
CPU=1200m MEM=100M create guaranteed
report allowed
verify 'len(mems["pod1c0"]) == 8'

echo "Verify that pod0 containers were not raised to guarantee memory"
echo ""
vm-command "$CRI_RESMGR_OUTPUT | grep -A5 upward" && {
    pp mems
    error "upward raising detected! note that it may not match memory pinning..."
}

kubectl delete pod pod1 --now

# pod2: reserve a lot of memory from the root node, force raising to root
# every socket has 6G PMEM+DRAM, pod0 containers take 5G and pod2c0 take 2G
# => pessimistic max 7G will not fit to any socket
# => no memory grants can be given to any socket alone.
CPU=1200m MEM=2G create guaranteed

echo "Verify that pod0 containers were raised to guarantee memory"
echo ""
vm-command "$CRI_RESMGR_OUTPUT | grep -A5 upward" || {
    error "upward raising expected but not found!"
}
report allowed
pp mems
verify 'len(mems["pod2c0"]) == 8' \
       'len(mems["pod0c0"]) == 8' \
       'len(mems["pod0c1"]) == 8'
