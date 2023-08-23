CRI_RESMGR_OUTPUT="cat cri-resmgr.output.txt | tr -d '\0'"
CRI_RESMGR_ROTATE="echo > cri-resmgr.output.txt"

podno=0
kubectl delete pod --all --now --wait

# account for being done with test for the current pod
nextpod () {
    podno=$((podno+1))
}

# print current pod name
pod () {
    echo pod$podno
}

# print current container name, by default for current pod
container () {
    local _p _c
    case $# in
        0) _p=${podno}; _c=0;;
        1) _p=${podno}; _c=$1;;
        2) _p=$1; _c=$2;;
        *)
            _c=pod${1}c${2}; shift 2
            echo ${_c}_INVALID_WITH_EXTRA_${#}_ARGS_$(echo $* | tr -s ' ' '_')
            return 1
            ;;
    esac
    case $_p in
        +*|-*) _p=$((${podno}$_p));;
    esac
    echo pod${_p}c${_c}
}

# rotate cri-resmgr logs
rotate_log () {
    vm-command "$CRI_RESMGR_ROTATE"
}

###########################################################################
# test #1: squeeze multiple containers in every NUMA node
#
# We squeeze an increasing number of containers in all NUMA node pools
# in a loop. For every iteration we calculate the usable amount of CPU
# and memory based on the available number of NUMA nodes and the amount
# of CPU and memory per NUMA node. We use a conservative estimate for
# the amount of memory available per NUMA node because some of them will
# have a sizeable allocation by the kernel.
#

rotate_log

# use conservative estimate for available memory per node
PER_NODE_MEM=$((1500+4000))
PER_NODE_CPU=1000
PER_NODE_PMEM=1
NODE_COUNT_TOTAL=4
# All nodes have only a single CPU. Thus, with any (< 1000m) CPU reservation
# we'll have one node (#0) fully reserved for kube-system containers. Hence,
# our (usable) node count is one less than the total one.
NODE_COUNT=$((NODE_COUNT_TOTAL - 1))

for pernode in 2 3 4; do
    cpu=$(echo "scale=3;0.75*$PER_NODE_CPU/$pernode" | bc | cut -d '.' -f1)
    mem=$(echo "scale=3;0.75*$PER_NODE_MEM/$pernode" | bc | cut -d '.' -f1)
    CPU=${cpu}m MEM=${mem}Mi CONTCOUNT=$((pernode*NODE_COUNT)) create guaranteed

    echo "Verify that any pod's containers were not raised to guarantee memory"
    echo ""
    vm-command "$CRI_RESMGR_OUTPUT | grep upward" && {
        pp mems
        error "Unexpected memset upward expansion detected!"
    }

    echo "Verify that all containers are pinned to a single NUMA node"
    echo ""
    c=0; while [ "$c" -lt "$((pernode*NODE_COUNT))" ]; do
        verify "len(mems['$(container $c)']) == $((1+PER_NODE_PMEM))"
        c=$((c+1))
    done

    kubectl delete pod --all --now --wait

    nextpod
done

###########################################################################
# test #2: negative test for lifting containers upwards.
#
# This test first creates a pod that fits into a singe NUMA node. Then
# it creates a pod that allocates a negligible amount of memory from the
# root node (by asking for more CPU than a single NUMA node can provide).
# The allocation of this pod must not cause lifting pod0 containers'
# memory assignment upwards in the pool tree.
#

rotate_log

CPU=200m MEM=100M create guaranteed
report allowed
verify "len(mems['$(container 0)']) == 2"

nextpod

CPU=1200m MEM=100M create guaranteed
report allowed
verify "len(mems['$(container -1 0)']) == 2" \
       "len(mems['$(container 0)']) == 8"

echo "Verify that $(pod)'s containers were not raised to guarantee memory"
echo ""
vm-command "$CRI_RESMGR_OUTPUT | grep upward" && {
    pp mems
    error "Unexpected memset upward expansion detected!"
}

kubectl delete pod $(pod) --now --wait --ignore-not-found

nextpod

###########################################################################
# test #3: positive test for lifting containers upwards.
#
# This test creates two containers which both get their own socket and
# take > 50 % of their socket's mem. Then it reserves a lot of memory
# from the root node to force lifting one of the containers. Every socket
# has 6G PMEM+DRAM, one pods containers take 5G and the other take 2G.
# => pessimistic max 7G will not fit to any socket
# => no memory grants can be given to any socket alone.
#

CPU=200m MEM=5G CONTCOUNT=2 create guaranteed
report allowed
verify "len(mems['$(container 0)']) == 2" \
       "len(mems['$(container 1)']) == 2" \
       "mems['$(container 0)'] != mems['$(container 1)']"

nextpod

CPU=1200m MEM=2G create guaranteed

echo "Verify that $(pod)'s containers were raised to guarantee memory"
echo ""
vm-command "$CRI_RESMGR_OUTPUT | grep upward" || {
    error "Expected memset upward expansion not found!"
}
report allowed
pp mems
verify "len(mems['$(container 0)']) == 8" \
       "len(mems['$(container -1 0)']) == 8" \
       "len(mems['$(container -1 1)']) == 8"

