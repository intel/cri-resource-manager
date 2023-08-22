# Test resource allocation / free on different container exit and
# restart scenarios.

CONTCOUNT=1 CPU=1000m MEM=64M create guaranteed
report allowed
verify 'len(cpus["pod0c0"]) == 1'
pyexec 'assert "pod0c0" in allocations'

out '### Crash and restart pod0c0'
vm-command "kubectl get pods pod0"

vm-command "set -x; [[ -n \"\$(pgrep -f pod0c0)\" ]] && [[ \"\$(pgrep -f pod0c0 --oldest)\" != \"\$(pgrep -f pod0c0 --newest)\" ]]" || {
    command-error "There must be separate parent and child 'pod0c0' processes in order to run this test"
}

out '### Kill the root process in pod0c0. The container should get Restarted.'
vm-command "kill -KILL \$(pgrep -f pod0c0 --oldest)"
sleep 2
vm-command 'kubectl wait --for=condition=Ready pods/pod0'
vm-run-until --timeout 30 "pgrep -f pod0c0 > /dev/null 2>&1"
vm-command "kubectl get pods pod0"
report allowed
verify 'len(cpus["pod0c0"]) == 1'
pyexec 'assert "pod0c0" in allocations'

out '### Kill the child process in pod0c0. The root process exits with status 0, the container should get Completed.'
vm-command "kubectl get pods pod0"
vm-command "ps axf | grep pod0c0; echo newest: \$(pgrep -f pod0c0 --newest)"
vm-command "kill -KILL \$(pgrep -f pod0c0 --newest)"
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
