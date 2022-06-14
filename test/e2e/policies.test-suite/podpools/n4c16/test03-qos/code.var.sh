# Test all QoS class pods in a pool, reserved and shared CPUs.
# Verify that CFS CPU shares is set correctly in all cases.

vm-put-file "$HOST_PROJECT_DIR/scripts/testing/kube-cgroups" "/usr/local/bin/kube-cgroups"

verify-cpushare() {
    podXcY=$1
    expected_cgv1=$2
    expected_cgv2=$3
    vm-command "kube-cgroups -n . -c $podXcY -f 'cpu.(shares|weight)\$'"
    CPU_SHARES_WEIGHT=$(echo "$COMMAND_OUTPUT" | awk '/cpu.*:/{print $2}')
    if [ "$CPU_SHARES_WEIGHT" = "$expected_cgv1" ]; then
        echo "verified cpu.shares of $podXcY == $expected_cgv1"
    elif [ "$CPU_SHARES_WEIGHT" = "$expected_cgv2" ]; then
        echo "verified cpu.weight of $podXcY == $expected_cgv2"
    else
        echo "assertion failed when verifying $podXcY: got '$COMMAND_OUTPUT' expected 'cpu.shares=$expected_cgv1' or 'cpu.weight=$expected_cgv2'"
        exit 1
    fi
}

CPUREQ="" MEMREQ="" CPULIM="" MEMLIM="" POD_ANNOTATION=""

out "### Assigning BestEffort, Burstable and Guaranteed pods to the same (dualcpu) pool"
# pod0c0: besteffort
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" create podpools-busybox
# pod1c0: burstable
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=500m create podpools-busybox
# pod2c0: guaranteed
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=1 CPULIM=1 MEMREQ=100M MEMLIM=100M create podpools-busybox
report allowed

verify-cpushare pod0c0 2 1
verify-cpushare pod1c0 512 20
verify-cpushare pod2c0 1024 39

kubectl delete pods --all --now
reset counters

out "### Assigning BestEffort, Burstable and Guaranteed pods shared CPUs"
# pod0c0: besteffort
create podpools-busybox
# pod1c0: burstable
CPUREQ=500m create podpools-busybox
# pod2c0: guaranteed
CPUREQ=1 CPULIM=1 MEMREQ=100M MEMLIM=100M create podpools-busybox
report allowed

verify-cpushare pod0c0 2 1
verify-cpushare pod1c0 512 20
verify-cpushare pod2c0 1024 39

kubectl delete pods --all --now
reset counters

out "### Assigning BestEffort, Burstable and Guaranteed pods reserved CPUs"
# pod0c0: besteffort
namespace=kube-system create podpools-busybox
# pod1c0: burstable
namespace=kube-system CPUREQ=500m create podpools-busybox
# pod2c0: guaranteed
namespace=kube-system CPUREQ=1 CPULIM=1 MEMREQ=100M MEMLIM=100M create podpools-busybox
report allowed

verify-cpushare pod0c0 2 1
verify-cpushare pod1c0 512 20
verify-cpushare pod2c0 1024 39

kubectl delete pods pod0 pod1 pod2 -n kube-system
