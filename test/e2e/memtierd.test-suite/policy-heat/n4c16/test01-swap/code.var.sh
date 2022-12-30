memtierd-setup

# Create swap
zram-swap off
zram-swap 2G

# Test that 700M+ is swapped out from a process that has 1G of memory
# and writes actively 300M.
MEME_BS=1G MEME_BWC=1 MEME_BWS=300M memtierd-meme-start

MEMTIERD_YAML="
policy:
  name: heat
  config: |
    intervalms: 4000
    pids:
      - $MEME_PID
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

sleep 4
echo "waiting 700M+ to be paged out..."
while ! ( memtierd-command "stats"; grep PAGEOUT:0\.7[0-9][0-9] <<< $COMMAND_OUTPUT); do
    echo ":::$COMMAND_OUTPUT:::"
    sleep 1
done

echo "check swap status: correct pages have been paged out."
memtierd-command "swap -pid $MEME_PID -status"
grep " 7[0-9][0-9] " <<< $COMMAND_OUTPUT || {
    error "expected 7XX MB of swapped out"
}

memtierd-stop
memtierd-meme-stop
