terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/dyp-numa.cfg launch cri-resmgr

# pod0: besteffort, go to shared dynamic pool, make sure it still gets at least 1 CPU.
CPUREQ="" CPULIM="" MEMREQ="" MEMLIM=""
CONTCOUNT=1 create dyp-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 15'

# pod1: guaranteed, go to fit-in-numa dynamic pool, make sure it gets the CPU it requested.
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create dyp-busybox
report allowed
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod1c0"]) >= 1' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"])'

# pod2: guaranteed, go to fit-in-numa dynamic pool, make sure it gets the CPU it requested.
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create dyp-busybox
report allowed
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod1c0"]) >= 2' \
       'len(cpus["pod2c0"]) >= 2' \
       'cpus["pod1c0"] == cpus["pod2c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod2c0"])'

# pod3: guaranteed, go to fit-in-numa dynamic pool, make sure it gets the CPU it requested.
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create dyp-busybox
report allowed
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod1c0"]) >= 3' \
       'len(cpus["pod2c0"]) >= 3' \
       'len(cpus["pod3c0"]) >= 3' \
       'cpus["pod1c0"] == cpus["pod2c0"] == cpus["pod3c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod3c0"])'

# pod4: guaranteed, go to fit-in-numa dynamic pool, make sure it gets the CPU it requested.
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create dyp-busybox
report allowed
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod1c0"]) >= 4' \
       'len(cpus["pod2c0"]) >= 4' \
       'len(cpus["pod3c0"]) >= 4' \
       'len(cpus["pod4c0"]) >= 4' \
       'cpus["pod1c0"] == cpus["pod2c0"] == cpus["pod3c0"] == cpus["pod4c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod4c0"])'

# pod5: besteffort, no CPU request, should fit into the shared dynamic pool.
CPUREQ="" CPULIM="" MEMREQ="" MEMLIM=""
CONTCOUNT=1 create dyp-busybox
report allowed
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod1c0"]) >= 4' \
       'len(cpus["pod2c0"]) >= 4' \
       'len(cpus["pod3c0"]) >= 4' \
       'len(cpus["pod4c0"]) >= 4' \
       'len(cpus["pod5c0"]) >= 1' \
       'cpus["pod1c0"] == cpus["pod2c0"] == cpus["pod3c0"] == cpus["pod4c0"]' \
       'cpus["pod0c0"] == cpus["pod5c0"]'

# Leave only one guaranteed container to the fit-in-numa dynamic pool.
kubectl delete pods pod1 pod2 pod3 --now
report allowed
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod4c0"]) >= 1' \
       'len(cpus["pod5c0"]) >= 1' \
       'cpus["pod0c0"] == cpus["pod5c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod4c0"])'

# Leave only bestefforts to the dynamic pool.
kubectl delete pods pod4 --now
report allowed
verify 'len(cpus["pod0c0"]) >= 1' \
       'len(cpus["pod5c0"]) >= 1' \
       'cpus["pod0c0"] == cpus["pod5c0"]'

terminate cri-resmgr
launch cri-resmgr
