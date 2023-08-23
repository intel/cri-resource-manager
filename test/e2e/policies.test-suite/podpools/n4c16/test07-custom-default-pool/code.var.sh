# Launch cri-resmgr with a custom default pool and many highperf
# pools. The CPUs in the custom default pool are disjoint from CPUs in
# the reserved pool. 100 % of remaining CPUs are allocated to highperf
# pools.
terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/podpools-custom-default.cfg launch cri-resmgr

cleanup() {
    ( kubectl delete pods --all --now --wait )
    ( kubectl delete pod -n kube-system pod0c-mysystem --now --wait --ignore-not-found )
    ( kubectl delete namespace daemons --now --wait --ignore-not-found )
}

cleanup

namespace=kube-system NAME=pod0c-mysystem CONTCOUNT=2 create podpools-busybox
kubectl create namespace daemons
namespace=daemons NAME=pod0c-mydaemon CONTCOUNT=2 create podpools-busybox
report allowed
verify 'len(cpus["pod0c-mysystemc0"]) == 1' \
       'len(cpus["pod0c-mydaemonc0"]) == 3' \
       'disjoint_sets(cpus["pod0c-mysystemc0"], cpus["pod0c-mydaemonc0"])'

NAME=pod1c-highperf POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: highperf" CPUREQ=2 CPULIM=2 MEMREQ="" MEMLIM="" create podpools-busybox
NAME=pod2c-highperf POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: highperf" CPUREQ=2 CPULIM=2 MEMREQ="" MEMLIM="" create podpools-busybox
NAME=pod3c-highperf POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: highperf" CPUREQ=2 CPULIM=2 MEMREQ="" MEMLIM="" create podpools-busybox
NAME=pod4c-highperf POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: highperf" CPUREQ=2 CPULIM=2 MEMREQ="" MEMLIM="" create podpools-busybox
report allowed
verify 'len(cpus["pod1c-highperfc0"]) == 2' \
       'len(cpus["pod2c-highperfc0"]) == 2' \
       'len(cpus["pod3c-highperfc0"]) == 2' \
       'len(cpus["pod4c-highperfc0"]) == 2' \
       'disjoint_sets(cpus["pod1c-highperfc0"], cpus["pod2c-highperfc0"], cpus["pod3c-highperfc0"], cpus["pod4c-highperfc0"])'

cleanup
vm-command "cat < cri-resmgr.output.txt > cri-resmgr-podpools-single-pool.output.txt"
terminate cri-resmgr
launch cri-resmgr
