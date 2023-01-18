TMPDIR=/tmp/memtierd-e2e-$(basename "$TEST_DIR")
TMPSHELL=$TMPDIR/shell.output.txt
TMPMEMCMD=$TMPDIR/memtierd-command.output.txt
vm-command "mkdir -p '$TMPDIR'; rm -f '$TMPSHELL' '$TMPMEMCMD'"

memtierd-setup

# # Test that 700M+ is swapped out from a process that has 1G of memory
# # and writes actively 300M.
# MEME_CGROUP=e2e-meme MEME_BS=1G MEME_BWC=1 MEME_BWS=300M memtierd-meme-start

MEMTIERD_YAML="
routines:
  - name: statactions
    config: |
      intervalms: 1000
      intervalcommand: ['sh', '-c', 'date +%F-%T >> $TMPSHELL']
policy:
  name: stub
"
memtierd-start
sleep 4
memtierd-stop
vm-command "wc -l $TMPSHELL | awk '{print \$1}'"
[[ "$COMMAND_OUTPUT" -gt 2 ]] || {
    command-error "expected more than 2 lines in $TMPSHELL"
}

MEMTIERD_YAML="
routines:
  - name: statactions
    config: |
      intervalms: 200
      intervalcommandrunner: memtier
      intervalcommand: ['stats', '-t', 'events']
policy:
  name: stub
"
memtierd-start
sleep 4
memtierd-stop
vm-command "grep RoutineStatActions.command.memtier $MEMTIERD_OUTPUT | tail -n 1"
[[ $(awk '{print $1}' <<< "$COMMAND_OUTPUT") -gt 15 ]] || {
    command-error "at least 15 memtier command executions expected"
}
