terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-numa.cfg launch cri-resmgr

# pod0: besteffort, make sure it still gets at least 1 CPU
CPUREQ="" CPULIM="" MEMREQ="" MEMLIM=""
CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 1'

# pod1: guaranteed, make sure it gets the CPU it requested.
# The configuration does not prefer creating new balloons,
# so pod0 and pod1 should be placed in the same balloon.
# Sum of their CPU requests is 1, so they should actually
# run on the same CPU.
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod1c0"]) == 1' \
       'cpus["pod0c0"] == cpus["pod1c0"]'

# pod2: guaranteed, make sure it gets the CPU it requested.
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 2' \
       'len(cpus["pod1c0"]) == 2' \
       'len(cpus["pod2c0"]) == 2' \
       'cpus["pod0c0"] == cpus["pod1c0"] == cpus["pod2c0"]'

# pod3: guaranteed, make sure it gets the CPU it requested.
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 3' \
       'len(cpus["pod1c0"]) == 3' \
       'len(cpus["pod2c0"]) == 3' \
       'len(cpus["pod3c0"]) == 3' \
       'cpus["pod0c0"] == cpus["pod1c0"] == cpus["pod2c0"] == cpus["pod3c0"]'

# pod4: guaranteed, fill up a balloon to the MaxCPU
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 4' \
       'len(cpus["pod1c0"]) == 4' \
       'len(cpus["pod2c0"]) == 4' \
       'len(cpus["pod3c0"]) == 4' \
       'len(cpus["pod4c0"]) == 4' \
       'cpus["pod0c0"] == cpus["pod1c0"] == cpus["pod2c0"] == cpus["pod3c0"] == cpus["pod4c0"]'

# pod5: besteffort, no CPU request, should fit into the full balloon
CPUREQ="" CPULIM="" MEMREQ="" MEMLIM=""
CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 4' \
       'len(cpus["pod1c0"]) == 4' \
       'len(cpus["pod2c0"]) == 4' \
       'len(cpus["pod3c0"]) == 4' \
       'len(cpus["pod4c0"]) == 4' \
       'len(cpus["pod5c0"]) == 4' \
       'cpus["pod0c0"] == cpus["pod1c0"] == cpus["pod2c0"] == cpus["pod3c0"] == cpus["pod4c0"] == cpus["pod5c0"]'

# pod6: guaranteed, start filling new balloon
CPUREQ="1" CPULIM="1" MEMREQ="50M" MEMLIM="50M"
CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 4' \
       'len(cpus["pod1c0"]) == 4' \
       'len(cpus["pod2c0"]) == 4' \
       'len(cpus["pod3c0"]) == 4' \
       'len(cpus["pod4c0"]) == 4' \
       'len(cpus["pod5c0"]) == 4' \
       'len(cpus["pod6c0"]) == 1' \
       'cpus["pod0c0"] == cpus["pod1c0"] == cpus["pod2c0"] == cpus["pod3c0"] == cpus["pod4c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod6c0"])'

# Leave only one guaranteed container to the first balloon.
kubectl delete pods pod1 pod2 pod3 --now --wait --ignore-not-found
report allowed
verify 'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod4c0"]) == 1' \
       'len(cpus["pod5c0"]) == 1' \
       'len(cpus["pod6c0"]) == 1' \
       'cpus["pod0c0"] == cpus["pod4c0"] == cpus["pod5c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod6c0"])'

# Leave only bestefforts to the first balloon. Make sure they still
# have a CPU.
kubectl delete pods pod4 --now --wait --ignore-not-found
report allowed
verify 'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod5c0"]) == 1' \
       'len(cpus["pod6c0"]) == 1' \
       'cpus["pod0c0"] == cpus["pod5c0"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod6c0"])'

terminate cri-resmgr
launch cri-resmgr
