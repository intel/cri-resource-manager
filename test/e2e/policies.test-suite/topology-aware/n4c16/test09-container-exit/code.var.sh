# Test resource allocation / free on different container exit and
# restart scenarios.

# Make sure all the pods in default namespace are cleared so we get a fresh start
#kubectl delete pods --all --now

# Cleanup kernel commandline, otherwise isolcpus will affect CPU
# pinning and cause false negatives from other tests on this VM.
# This can happen if test08-isolcpus failed and we are re-running
# the tests from the start.
vm-command "grep isolcpus /proc/cmdline" && {
    vm-del-kernel-cmdline-arg "isolcpus=8,9"
    vm-force-restart
    vm-command "grep isolcpus /proc/cmdline" && {
	error "failed to clean up isolcpus kernel commandline parameter"
    }
    echo "isolcpus removed from kernel commandline"
    vm-command "systemctl restart kubelet"
    vm-wait-process --timeout 120 kube-apiserver

    # Do a fresh start
    terminate cri-resmgr
    launch cri-resmgr
    sleep 2
}

CONTCOUNT=1 CPU=1000m MEM=64M create guaranteed
report allowed
verify 'len(cpus["pod0c0"]) == 1'
pyexec 'assert "pod0c0" in allocations'

out '### Crash and restart pod0c0'
vm-command "kubectl get pods pod0"
vm-command "kill -KILL \$(pgrep -f 'echo pod0c0')"
sleep 2
vm-command 'kubectl wait --for=condition=Ready pods/pod0'
vm-run-until --timeout 30 "pgrep -f 'echo pod0c0' > /dev/null 2>&1"
vm-command "kubectl get pods pod0"
report allowed
verify 'len(cpus["pod0c0"]) == 1'
pyexec 'assert "pod0c0" in allocations'

out '### Exit and complete pod0c0 by killing "sleep inf"'
out '### => sh (the init process in the container) will exit with status 0'
vm-command "kubectl get pods pod0"
vm-command "kill -KILL \$(pgrep --parent \$(pgrep -f 'echo pod0c0') sleep)"
sleep 2
vm-command "kubectl get pods pod0"
# pod0c0 process is not on vm anymore
verify '"pod0c0" not in cpus'
# pod0c0 is not allocated any resources on CRI-RM
( verify '"pod0c0" not in allocations' ) || {
    # pretty-print allocations contents
    pp allocations
    error "pod0c0 expected to disappear from allocations"
}
