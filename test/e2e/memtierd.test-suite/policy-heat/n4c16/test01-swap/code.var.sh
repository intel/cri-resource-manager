memtierd-setup

# Create swap
zram-install
zram-swap off
zram-swap 2G

# Test that 700M+ is swapped out from a process that has 1G of memory
# and writes actively 300M.
MEME_CGROUP=e2e-meme MEME_BS=1G MEME_BWC=1 MEME_BWS=300M memtierd-meme-start

MEMTIERD_YAML="
policy:
  name: heat
  config: |
    intervalms: 4000
    pidwatcher:
      name: cgroups
      config: |
        cgroups:
          - /sys/fs/cgroup/e2e-meme
    heatnumas:
      0: [-1]
    heatmap:
      heatmax: 0.01
      heatretention: 0
      heatclasses: 5
    tracker:
      name: softdirty
      config: |
        pagesinregion: 512
        maxcountperregion: 0
        scanintervalms: 500
        regionsupdatems: 0
    mover:
      intervalms: 20
      bandwidth: 1000
"
memtierd-start

next-round() {
    local round_counter_var=$1
    local round_counter_val=${!1}
    local round_counter_max=$2
    local round_delay=$3
    if [[ "$round_counter_val" -ge "$round_counter_max" ]]; then
        return 1
    fi
    eval "$round_counter_var=$(($round_counter_val + 1))"
    sleep $round_delay
    return 0
}

match-pageout() {
    local pageout_regexp=$1
    round_number=0
    while ! ( memtierd-command "stats -t process_madvise -f csv | awk -F, \"{print \\\$6}\""; grep ${pageout_regexp} <<< $COMMAND_OUTPUT); do
        echo "grep PAGEOUT value matching ${pageout_regexp} not found"
        next-round round_number 5 1 || {
            error "timeout: memtierd did not expected amount of memory"
        }
    done
}


sleep 4
round_number=0
echo "waiting 700M+ to be paged out..."
match-pageout "0\.7[0-9][0-9]"

echo "check swap status: correct pages have been paged out."
memtierd-command "swap -pid $MEME_PID -status"
grep " 7[0-9][0-9] " <<< $COMMAND_OUTPUT || {
    error "expected 7XX MB of swapped out"
}

echo "stop meme, expect all memory to be paged out"
vm-command "kill -STOP ${MEME_PID}"
match-pageout "1\.[0-2]"

echo "continue meme, expect 300 MB to be swapped in by OS"
vm-command "kill -CONT ${MEME_PID}"
sleep 4
memtierd-command "swap -pid $MEME_PID -status"
grep " 7[0-9][0-9] " <<< $COMMAND_OUTPUT || {
    error "expected 7XX MB of swapped out"
}

echo "stop meme again, verify that all memory gets paged out again"
vm-command "kill -STOP ${MEME_PID}"
match-pageout "1\.[3-5]"

memtierd-stop
memtierd-meme-stop
