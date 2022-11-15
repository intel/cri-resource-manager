terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-isolated.cfg cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr

verify-metrics-has-line 'balloon="isolated-pods\[0\]"'
verify-metrics-has-line 'balloon="isolated-pods\[1\]"'
verify-metrics-has-no-line 'balloon="isolated-pods\[2\]"'

# pod0: besteffort
CPUREQ="" CPULIM="" MEMREQ="" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: isolated-pods"
CONTCOUNT=2 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod0c1"]) == 1' \
       'cpus["pod0c0"] == cpus["pod0c1"]'
# Even if the isolated balloon type has PreferNewBalloons=1, adding
# this pod0 or pod1 must not create a new balloon because existing
# empty balloons should be filled first.
verify-metrics-has-line 'balloon="isolated-pods\[0\]"'
verify-metrics-has-line 'balloon="isolated-pods\[1\]"'
verify-metrics-has-no-line 'balloon="isolated-pods\[2\]"'

# pod1: guaranteed
CPUREQ="600m" CPULIM="600m" MEMREQ="100M" MEMLIM="100M"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: isolated-pods"
CONTCOUNT=2 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod0c1"]) == 1' \
       'len(cpus["pod1c0"]) == 2' \
       'len(cpus["pod1c1"]) == 2' \
       'cpus["pod0c0"] == cpus["pod0c1"]' \
       'cpus["pod1c0"] == cpus["pod1c1"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"])'
verify-metrics-has-line 'balloon="isolated-pods\[0\]"'
verify-metrics-has-line 'balloon="isolated-pods\[1\]"'
verify-metrics-has-no-line 'balloon="isolated-pods\[2\]"'

# pod2: burstable
CPUREQ="100m" CPULIM="200m"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: isolated-pods"
CONTCOUNT=2 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod0c1"]) == 1' \
       'len(cpus["pod1c0"]) == 2' \
       'len(cpus["pod1c1"]) == 2' \
       'len(cpus["pod2c0"]) == 1' \
       'len(cpus["pod2c1"]) == 1' \
       'cpus["pod0c0"] == cpus["pod0c1"]' \
       'cpus["pod1c0"] == cpus["pod1c1"]' \
       'cpus["pod2c0"] == cpus["pod2c1"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"])'
verify-metrics-has-line 'balloon="isolated-pods\[0\]"'
verify-metrics-has-line 'balloon="isolated-pods\[1\]"'
verify-metrics-has-line 'balloon="isolated-pods\[2\]"'
verify-metrics-has-no-line 'balloon="isolated-pods\[3\]"'

# pod3: isolated containers
CPUREQ="" CPULIM="" MEMREQ="" MEMLIM=""
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: isolated-ctrs"
CONTCOUNT=4 create balloons-busybox
report allowed
verify 'len(cpus["pod0c0"]) == 1' \
       'len(cpus["pod0c1"]) == 1' \
       'len(cpus["pod1c0"]) == 2' \
       'len(cpus["pod1c1"]) == 2' \
       'len(cpus["pod2c0"]) == 1' \
       'len(cpus["pod2c1"]) == 1' \
       'len(cpus["pod3c0"]) == 1' \
       'len(cpus["pod3c1"]) == 1' \
       'len(cpus["pod3c2"]) == 1' \
       'len(cpus["pod3c3"]) == 1' \
       'cpus["pod0c0"] == cpus["pod0c1"]' \
       'cpus["pod1c0"] == cpus["pod1c1"]' \
       'cpus["pod2c0"] == cpus["pod2c1"]' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"])' \
       'disjoint_sets(cpus["pod3c0"], cpus["pod3c1"], cpus["pod3c2"], cpus["pod3c3"])' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"], cpus["pod3c0"], cpus["pod3c1"], cpus["pod3c2"], cpus["pod3c3"])'
verify-metrics-has-line 'balloon="isolated-pods\[0\]"'
verify-metrics-has-line 'balloon="isolated-pods\[1\]"'
verify-metrics-has-line 'balloon="isolated-pods\[2\]"'
verify-metrics-has-no-line 'balloon="isolated-pods\[3\]"'
verify-metrics-has-line 'balloon="isolated-ctrs\[0\]"'
verify-metrics-has-line 'balloon="isolated-ctrs\[1\]"'
verify-metrics-has-line 'balloon="isolated-ctrs\[2\]"'
verify-metrics-has-line 'balloon="isolated-ctrs\[3\]"'

terminate cri-resmgr
launch cri-resmgr
