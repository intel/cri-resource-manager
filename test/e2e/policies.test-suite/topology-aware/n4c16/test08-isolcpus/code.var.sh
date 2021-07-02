vm-command "grep isolcpus=8,9 /proc/cmdline" || {
    vm-set-kernel-cmdline "isolcpus=8,9"
    vm-reboot
    vm-command "grep isolcpus=8,9 /proc/cmdline" || {
        error "failed to set isolcpus kernel commandline parameter"
    }
    launch cri-resmgr
    vm-command "systemctl restart kubelet"
    sleep 1
    vm-wait-process --timeout 120 kube-apiserver
    vm-run-until --timeout 120 "kubectl get node"
}

CONTCOUNT=1

# pod0: opt-in isolated CPUs
ANNOTATIONS='prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"'
CPU=1 create guaranteed-annotated
report allowed
verify "cpus['pod0c0'] == {'cpu08'} or cpus['pod0c0'] == {'cpu09'}" \
       "mems['pod0c0'] == {'node2'}"

# pod1: opt-out isolated CPUs
ANNOTATIONS='prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "false"'
CPU=1 create guaranteed-annotated
report allowed
verify "disjoint_sets(cpus['pod1c0'], {'cpu08', 'cpu09'})"

# pod2: without annotation CPU=1 guaranteed pod is eligible to run on isolated CPUs
ANNOTATIONS=''
CPU=1 create guaranteed-annotated
report allowed
verify "cpus['pod0c0'] == {'cpu08'} or cpus['pod0c0'] == {'cpu09'}" \
       "cpus['pod2c0'] == {'cpu08'} or cpus['pod2c0'] == {'cpu09'}" \
       "disjoint_sets(cpus['pod0c0'], cpus['pod2c0'])" \
       "mems['pod0c0'] == {'node2'}" \
       "mems['pod2c0'] == {'node2'}"

# free isolated (and all other) cpus
kubectl delete pods --all --now

# pod3: opt-in isolated CPUs, take all of them
ANNOTATIONS='prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"'
CPU=2000m create guaranteed-annotated
report allowed
verify "cpus['pod3c0'] == {'cpu08', 'cpu09'}" \
       "len(cpus['pod3c0']) == 2"

# free isolated cpus
kubectl delete pods --all --now

# pod4: opt-in isolated CPUs but require a fraction more CPUs than there are isolated CPUs
ANNOTATIONS=('prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"'
             'prefer-shared-cpus.cri-resource-manager.intel.com/pod: "false"')
CPU=2500m create guaranteed-annotated
report allowed
verify "'cpu08' in cpus['pod4c0'] and 'cpu09' in cpus['pod4c0']" \
       "len(cpus['pod4c0']) == 4"

# free isolated cpus
kubectl delete pods --all --now

# pod5: opt-in isolated CPUs but require a fraction less CPUs than there are isolated CPUs
ANNOTATIONS=('prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"'
             'prefer-shared-cpus.cri-resource-manager.intel.com/pod: "false"')
CPU=1500m create guaranteed-annotated
report allowed
verify "'cpu08' in cpus['pod5c0'] or 'cpu09' in cpus['pod5c0']" \
       "'cpu10' in cpus['pod5c0'] and 'cpu11' in cpus['pod5c0']" \
       "len(cpus['pod5c0']) == 3"

# free isolated cpus
kubectl delete pods --all --now

# pod6: opt-in isolated CPUs but require a full CPU more than there
# are isolated CPUs
ANNOTATIONS=('prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"'
             'prefer-shared-cpus.cri-resource-manager.intel.com/pod: "false"')
CPU=3000m create guaranteed-annotated
report allowed
verify "len(cpus['pod6c0']) == 3" \
       "disjoint_sets(cpus['pod6c0'], {'cpu08', 'cpu09'})" \
       "len(mems['pod6c0']) == 1"

# pod7: sub-core is never eligble for isolated CPUs, even if annotated
# to opt-in.
ANNOTATIONS=('prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"'
             'prefer-shared-cpus.cri-resource-manager.intel.com/pod: "false"')
CONTCOUNT=4 CPU=200m create guaranteed-annotated
report allowed
verify "disjoint_sets(set.union(cpus['pod7c0'], cpus['pod7c1'], cpus['pod7c2'], cpus['pod7c3']), {'cpu08', 'cpu09'})" \
       "len(cpus['pod7c0']) >= 2" \
       "len(cpus['pod7c1']) >= 2" \
       "len(cpus['pod7c2']) >= 2" \
       "len(cpus['pod7c3']) >= 2"

# Cleanup kernel commandline, otherwise isolcpus will affect CPU
# pinning and cause false negatives from other tests on this VM.
vm-set-kernel-cmdline ""
vm-reboot
vm-command "grep isolcpus /proc/cmdline" && {
    error "failed to clean up isolcpus kernel commandline parameter"
}
echo "isolcpus removed from kernel commandline"
launch cri-resmgr
vm-command "systemctl restart kubelet"
vm-wait-process --timeout 120 kube-apiserver
