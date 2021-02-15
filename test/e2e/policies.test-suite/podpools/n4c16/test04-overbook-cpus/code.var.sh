# Test CPU request warnings and errors:
# - Overbooked CPU sets
# - Bad CPU requests: mismatch between pool CPUs per pod and container CPU requests

CRI_RESMGR_OUTPUT="cat cri-resmgr.output.txt"

# pod0: overbook with single burstable pod and container
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=2900m CPULIM="" MEMREQ="" MEMLIM="" create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^E.*overbooked.*(2899|2900)m'" || error "missing overbook warning"
kubectl delete pods --all --now

# pod1: overbook with single burstable pod with two containers
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=1050m CPULIM="" MEMREQ="" MEMLIM="" CONTCOUNT=2 create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^E.*overbooked.*2100m'" || error "missing overbook warning"
kubectl delete pods --all --now

# pod2, pod3: overbook with two guaranteed pods, one container in each pod
n=2 POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=1001m MEMREQ=100M CPULIM=1001m MEMLIM=100M create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^E.*overbooked.*2002m'" || error "missing overbook warning"
kubectl delete pods --all --now

# pod4, pod5: no overbooking with exact CPUs guaranteed + besteffort pod
terminate cri-resmgr # restart to clear log
launch cri-resmgr
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=1000m CPULIM=1000m MEMREQ=100M MEMLIM=100M CONTCOUNT=2 create podpools-busybox
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ="" CPULIM="" MEMREQ="" MEMLIM="" create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^E.*overbooked'" && error "overbook warning with maximum allowed load"
kubectl delete pods --all --now
# podpools logs misaligned CPU requests after pod deletion
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^E.*bad CPU requests:.*pod4.* requested 2000 mCPUs.* 666 mCPUs'" || error "bad CPU request from pod4 expected but not found"
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^E.*bad CPU requests:.*pod5.* requested 0 mCPUs.* 666 mCPUs'" || error "bad CPU request from pod5 expected but not found"

# pod6: request 4 * 167 mCPU, that is almost required 666 mCPU. Should not be bad CPU request
POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=167m CPULIM="" MEMREQ="" MEMLIM="" CONTCOUNT=4 create podpools-busybox
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^E.*bad CPU requests:.*pod6'" && error "pod6 CPU request was ok, but 'bad CPU request' error found"

kubectl delete pods --all --now
