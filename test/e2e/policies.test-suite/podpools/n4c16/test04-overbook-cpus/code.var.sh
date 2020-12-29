# Test overbooking CPUs in pod pools

CRI_RESMGR_OUTPUT="cat cri-resmgr.output.txt"

# pod0: overbook with single burstable pod and container
POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=2900m MEMREQ="" CPULIM="" MEMLIM="" create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^W.*overbooked.*6-7.*(2899|2900)m'" || error "missing overbook warning"
kubectl delete pods --all --now

# pod1: overbook with single burstable pod with two containers
POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=1050m MEMREQ="" CPULIM="" MEMLIM="" CONTCOUNT=2 create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^W.*overbooked.*6-7.*2100m'" || error "missing overbook warning"
kubectl delete pods --all --now

# pod2, pod3: overbook with two guaranteed pods, one container in each pod
n=2 POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=1001m MEMREQ=100M CPULIM=1001m MEMLIM=100M create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^W.*overbooked.*6-7.*2002m'" || error "missing overbook warning"
kubectl delete pods --all --now

# pod4, pod5: no overbooking with exact CPUs guaranteed + besteffort pod
terminate cri-resmgr # restart to clear log
launch cri-resmgr
POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ=2000m MEMREQ=100M CPULIM=2000m MEMLIM=100M create podpools-busybox
POD_ANNOTATION="pooltype.podpools.cri-resource-manager.intel.com: dualcpu" CPUREQ="" MEMREQ="" CPULIM="" MEMLIM="" create podpools-busybox
report allowed
vm-command "$CRI_RESMGR_OUTPUT | grep -E '^W.*overbooked'" && error "overbook warning with maximum allowed load"
kubectl delete pods --all --now
