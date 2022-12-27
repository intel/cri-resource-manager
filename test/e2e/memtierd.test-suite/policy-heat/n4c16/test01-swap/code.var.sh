memtierd-setup

zram-swap off
zram-swap 2G

memtierd-meme-start

vm-command "pgrep meme"
echo "meme pid: $MEME_PID"

MEMTIERD_YAML="
policy:
  name: heat
  config: |
    intervalms: 10000
    pids:
      - $MEME_PID
    heatnumas:
      0: [-1]
    heatmap:
      heatmax: 0.01
      heatretention: 0.8
      heatclasses: 5
    tracker:
      name: softdirty
      config: |
        pagesinregion: 256
        maxcountperregion: 0
        scanintervalms: 1000
        regionsupdatems: 0
    mover:
      intervalms: 20
      bandwidth: 200
"
memtierd-start

sleep 5
memtierd-command "stats"
vm-command "cat memtierd.output.txt"
sleep 1
memtierd-command "policy -dump heatgram"

memtierd-stop
memtierd-meme-stop
