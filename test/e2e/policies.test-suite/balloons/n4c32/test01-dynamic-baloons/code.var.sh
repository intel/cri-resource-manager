terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-dynamic.cfg cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr

# pod0-pod7: create 8 balloons, where each lands on a different NUMA node.
# Each balloon (except one that lands on the NUMA node with reserved CPUs)
# has 1 shared CPU at the most since a NUMA node has 4 CPUs and a pod is
# requesting 1 CPU. Only one of the balloon that using NUMA node with
#reserved CPU has 0 shared CPUs.
CPUREQLIM="3"
INITCPUREQLIM="100m-100m 100m-100m 100m-100m"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
n=8 create multicontainerpod
verify-metrics-has-line 'balloon="dynamic\[0\]".*cpus_count="3"*'
verify-metrics-has-line 'balloon="dynamic\[1\]".*cpus_count="3"*'
verify-metrics-has-line 'balloon="dynamic\[2\]".*cpus_count="3"*'
verify-metrics-has-line 'balloon="dynamic\[3\]".*cpus_count="3"*'
verify-metrics-has-line 'balloon="dynamic\[4\]".*cpus_count="3"*'
verify-metrics-has-line 'balloon="dynamic\[5\]".*cpus_count="3"*'
verify-metrics-has-line 'balloon="dynamic\[6\]".*cpus_count="3"*'
verify-metrics-has-line 'balloon="dynamic\[7\]".*cpus_count="3"*'
verify-metrics-has-no-line 'cpus_count="4"'
verify-metrics-has-line 'sharedidlecpus_count="1"'
verify-metrics-has-line 'cpus_allowed_count="4"'
verify-metrics-has-line 'sharedidlecpus_count="0"'
verify-metrics-has-line 'cpus_allowed_count="3"'
verify-metrics-has-no-line 'sharedidlecpus_count="2"'
verify-metrics-has-no-line 'cpus_allowed_count="5"'
verify 'disjoint_sets(nodes["pod0c0"], nodes["pod1c0"], nodes["pod2c0"], nodes["pod3c0"], nodes["pod4c0"], nodes["pod5c0"], nodes["pod6c0"], nodes["pod7c0"])' \
       'len(nodes["pod0c0"]) == len(nodes["pod1c0"]) == len(nodes["pod2c0"]) == \
        len(nodes["pod3c0"]) == len(nodes["pod4c0"]) == len(nodes["pod5c0"]) == \
        len(nodes["pod6c0"]) == len(nodes["pod7c0"]) == 1'

# pod8: Add one more pod with 2 CPUs to inflate over NUMAs nodes, which should cross
# the NUMA node boundaries but not the die boundaries. Because two NUMA nodes can offer
# 2 CPUs in total. 
CPUREQLIM="2"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
create multicontainerpod
verify-metrics-has-line 'cpus_count="5"'
verify-metrics-has-line 'sharedidlecpus="",sharedidlecpus_count="0"'
verify-metrics-has-line 'sharedidlecpus_count="1"'
verify 'len(nodes["pod8c0"])==2' \
       'len(dies["pod8c0"])==1' \
       'len(packages["pod8c0"])==1'
kubectl delete pod pod8 --now --wait --ignore-not-found
verify-metrics-has-no-line 'cpus_count="5"'

# pod9: Add one more pod with 4 CPUs to inflate over dies, which should cross
# the NUMA node boundaries as well as dies boundaries. Since 2 dies under the
# same package can offer 4 CPUs, we should not cross the package boundaries.
CPUREQLIM="4"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
create multicontainerpod
verify 'len(nodes["pod9c0"])==4' \
       'len(dies["pod9c0"])==2' \
       'len(packages["pod9c0"])==1'
kubectl delete pod pod9 --now --wait --ignore-not-found
verify 'disjoint_sets(nodes["pod0c0"], nodes["pod1c0"], nodes["pod2c0"], nodes["pod3c0"], nodes["pod4c0"], nodes["pod5c0"], nodes["pod6c0"], nodes["pod7c0"])' \

# pod9: Add one more pod with 7 CPUs to inflate over packages, which should cross
# NUMA node, dies and package boundaries. At this point, there is no free CPUs
# left on the host, so no shared CPUs.
CPUREQLIM="6 1"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
create multicontainerpod
verify 'len(nodes["pod10c0"])==7' \
       'len(dies["pod10c0"])==4' \
       'len(packages["pod10c0"])==2'
verify-metrics-has-line 'sharedidlecpus="",sharedidlecpus_count="0"'
verify-metrics-has-no-line 'sharedidlecpus_count="1"'

# pod0, pod9 deflate. This should free up 10 CPUs that will cause having
# shared CPUs available again.
kubectl delete pod pod10 --now --wait --ignore-not-found
kubectl delete pod pod0 --now --wait --ignore-not-found
verify-metrics-has-line 'sharedidlecpus_count="1"'
