# Make sure the system has DAMON support enabled.
vm-command "[ -d /sys/kernel/mm/damon ]" || {
    damon-idlepage-setup
    vm-command "[ -d /sys/kernel/mm/damon ]" || error "failed to setup damon"
}

memtierd-setup

MEME_CGROUP=e2e-meme MEME_BS=1G MEME_BWC=1 MEME_BWS=300M memtierd-meme-start

MEMTIERD_YAML="
policy:
  name: heat
  config: |
    intervalms: 4000
    pidwatcher:
      name: pidlist
      config: |
        pids:
          - $MEME_PID
    heatnumas:
      0: [0]
      1: [1]
      3: [2]
      4: [3]
    heatmap:
      heatmax: 0.01
      heatretention: 0.92
      heatclasses: 5
    tracker:
      name: damon
      config: |
        connection: bpftrace
        samplingus: 1000
        aggregationus: 200000
        regionsupdateus: 3000000
        mintargetregions: 128
        maxtargetregions: 8192
        interface: 0
        sysfsregionsmanager: 1
        filteraddressrangesizemax: 8192000
        kdamondslist: [3, 4, 5]
        nrkdamonds: 8
    mover:
      intervalms: 20
      bandwidth: 1000
"
memtierd-start
memtierd-stop
