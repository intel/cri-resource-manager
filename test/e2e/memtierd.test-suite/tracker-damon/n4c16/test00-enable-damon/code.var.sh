# Make sure the system has DAMON support enabled.
vm-command "[ -d /sys/kernel/mm/damon ]" || {
    damon-idlepage-setup
    vm-command "[ -d /sys/kernel/mm/damon ]" || error "failed to setup damon"
}
