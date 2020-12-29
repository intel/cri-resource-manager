# Test all QoS class pods in a pool, reserved and shared CPUs.
# Verify that CFS CPU shares is set correctly in all cases.

verify-cpushare() {
    podXcY=$1
    expected=$2
    vm-command "cputasks=\$(find /sys/fs/cgroup/cpu/ -name tasks -type f -print0 | xargs -0 grep -l \$(pgrep -f ${podXcY})); cpudir=\${cputasks%/*}; cat \$cpudir/cpu.shares"
    if [ "$COMMAND_OUTPUT" = "$expected" ]; then
        echo "verified cpu.share of $podXcY == $expected"
    else
        echo "assertion failed when verifying cpu.share of $podXcY: got '$COMMAND_OUTPUT' expected '$expected'"
        exit 1
    fi
}

CPUREQ="" MEMREQ="" CPULIM="" MEMLIM="" POD_ANNOTATION=""

out "### Assigning BestEffort, Burstable and Guaranteed pods to the same (dualcpu) pool"
# pod0c0: besteffort
POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" create podpools-busybox
# pod1c0: burstable
POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=500m create podpools-busybox
# pod2c0: guaranteed
POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=1 CPULIM=1 MEMREQ=100M MEMLIM=100M create podpools-busybox
report allowed

verify-cpushare pod0c0 2
verify-cpushare pod1c0 512
verify-cpushare pod2c0 1024

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

verify-cpushare pod0c0 2
verify-cpushare pod1c0 512
verify-cpushare pod2c0 1024

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

verify-cpushare pod0c0 2
verify-cpushare pod1c0 512
verify-cpushare pod2c0 1024

kubectl delete pods pod0 pod1 pod2 -n kube-system
