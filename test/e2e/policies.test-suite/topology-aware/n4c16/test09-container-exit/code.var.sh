# Test resource allocation / free on different container exit and
# restart scenarios.

CONTCOUNT=1 CPU=1000m MEM=64M create guaranteed
report allowed
verify 'len(cpus["pod0c0"]) == 1'
pyexec 'assert "pod0c0" in allocations'

out '### Crash and restart pod0c0'
vm-command "kubectl get pods pod0"
vm-command "kill -KILL \$(pgrep -f pod0c0)"
sleep 2
vm-command "kubectl get pods pod0"
report allowed
verify 'len(cpus["pod0c0"]) == 1'
pyexec 'assert "pod0c0" in allocations'

out '### Exit and complete pod0c0 by killing "sleep inf"'
out '### => sh (the init process in the container) will exit with status 0'
vm-command "kubectl get pods pod0"
vm-command "kill -KILL \$(pgrep --parent \$(pgrep -f pod0c0) sleep)"
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
