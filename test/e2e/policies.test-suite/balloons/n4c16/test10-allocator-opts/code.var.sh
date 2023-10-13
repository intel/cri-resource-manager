cleanup() {
    vm-command "kubectl delete pods --all --now --wait"
    return 0
}

cleanup

# Launch cri-resmgr with wanted metrics update interval and a
# configuration that opens the instrumentation http server.
terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-allocator-opts.cfg launch cri-resmgr

# pod0 in a 2-CPU balloon
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: policydefaults" CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cores["pod0c0"]) == 2' \
       'len(cpus["pod0c0"]) == 2'


# pod1 in a 2-CPU balloon
CPUREQ="100m" MEMREQ="100M" CPULIM="100m" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: topo1cores0" CONTCOUNT=1 create balloons-busybox
report allowed
verify 'len(cores["pod1c0"]) == 1' \
       'len(cpus["pod1c0"]) == 2'

# pod2: container 0 resizes first from 0 to 1, container 2 from 1 to 2 CPUs,
# use more cores
CPUREQ="1" MEMREQ="100M" CPULIM="1" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: topo1cores1" CONTCOUNT=2 create balloons-busybox
report allowed
verify 'len(cores["pod2c0"]) == 2' \
       'len(cpus["pod2c0"]) == 2' \
       'cpus["pod2c0"] == cpus["pod2c1"]'

# pod3: container 0 resizes first from 0 to 1, container 2 from 1 to 2 CPUs,
# pack tightly
CPUREQ="1" MEMREQ="100M" CPULIM="1" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: topo0cores0" CONTCOUNT=2 create balloons-busybox
report allowed
verify 'len(cores["pod3c0"]) == 1' \
       'len(cpus["pod3c0"]) == 2' \
       'cpus["pod3c0"] == cpus["pod3c1"]'

cleanup
