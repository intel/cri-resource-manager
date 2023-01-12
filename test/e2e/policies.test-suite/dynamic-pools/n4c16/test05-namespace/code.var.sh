terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/dyp-namespace.cfg launch cri-resmgr

cleanup() {
    vm-command \
        "kubectl delete pods -n e2e-a --all --now
         kubectl delete pods -n e2e-b --all --now
         kubectl delete pods -n e2e-c --all --now
         kubectl delete pods -n e2e-d --all --now
         kubectl delete pods --all --now
         kubectl delete namespace e2e-a
         kubectl delete namespace e2e-b
         kubectl delete namespace e2e-c
         kubectl delete namespace e2e-d"
    return 0
}
cleanup

kubectl create namespace e2e-a
kubectl create namespace e2e-b
kubectl create namespace e2e-c
kubectl create namespace e2e-d

# pod0: create in the default namespace, CPUREQ is nil, both containers go to shared dynamic pool.
CPUREQ=""
CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod0c0"] == cpus["pod0c1"]' \
       'len(cpus["pod0c0"]) == 15'

# pod1: create in the e2e-a namespace, CPUREQ is nil, both containers go to shared dynamic pool.
CPUREQ=""
namespace="e2e-a" CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod1c0"] == cpus["pod1c1"] == cpus["pod0c0"]' \
       'len(cpus["pod1c0"]) == 15' \

# pod2: create in the default namespace, CPUREQ is 2*2, both containers go to nsdyp dynamic pool.
CPUREQ="2" MEMREQ="100M" CPULIM="2" MEMLIM="100M"
CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod2c0"] == cpus["pod2c1"]' \
       'len(cpus["pod2c0"]) >= 4' \
       'disjoint_sets(cpus["pod2c0"], cpus["pod1c0"])' \
       'disjoint_sets(cpus["pod2c0"], cpus["pod0c0"])'

# pod3: create again in the default namespace, CPUREQ is 200m*2, both containers go to nsdyp dynamic pool.
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod3c0"] == cpus["pod3c1"] == cpus["pod2c0"]' \
       'len(cpus["pod3c0"]) >= 5' 

# pod4: create in the e2e-b namespace, CPUREQ is 2*2, both containers go to nsdyp dynamic pool.
CPUREQ="2" MEMREQ="100M" CPULIM="2" MEMLIM="100M"
namespace="e2e-b" CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod4c0"] == cpus["pod4c1"] == cpus["pod3c0"] == cpus["pod2c0"]' \
       'len(cpus["pod4c0"]) >= 9' 

# pod5: create in the e2e-c namespace, CPUREQ is 100m*2, both containers go to nsdyp dynamic pool.
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
namespace="e2e-c" CONTCOUNT=2 create dyp-busybox
report allowed
verify 'cpus["pod5c0"] == cpus["pod5c1"] == cpus["pod4c0"] == cpus["pod3c0"] == cpus["pod2c0"]' \
       'len(cpus["pod5c0"]) >= 9' 

cleanup
terminate cri-resmgr
launch cri-resmgr
