MEMTIERD_PORT=${MEMTIERD_PORT:-5555}

memtierd-setup() {
    memtierd-install
    memtierd-reset
}

memtierd-install() {
    if [[ "$reinstall_memtierd" == "1" ]] || ! vm-command "command -v memtierd"; then
        if [ -z "$binsrc" ] || [ "$binsrc" == "local" ]; then
            vm-put-file "$BIN_DIR/memtierd" "$prefix/bin/memtierd"
            vm-put-file "$BIN_DIR/meme" "$prefix/bin/meme"
        else
            error "memtierd-install: unsupported binsrc: '$binsrc'"
        fi
    fi
}

memtierd-reset() {
    vm-command "killall -KILL memtierd meme socat"
}

memtierd-start() {
    vm-pipe-to-file "memtierd.yaml" <<< "${MEMTIERD_YAML}"
    vm-command "nohup sh -c 'socat tcp4-listen:${MEMTIERD_PORT},fork,reuseaddr - | memtierd -config memtierd.yaml -debug' > memtierd.output.txt 2>&1 & sleep 2; cat memtierd.output.txt"
}

memtierd-stop() {
    memtierd-command "q"
    sleep 1
    vm-command "killall -KILL memtierd"
}

memtierd-command() {
    vm-command "echo '$1' | socat - tcp4:localhost:${MEMTIERD_PORT}"
}

memtierd-meme-start() {
    vm-command "nohup meme -bs 1G -brc 0 -bwc 1 -bws 128M -bwo 256M -ttl 1h < /dev/null > meme.output.txt 2>&1 & sleep 2; cat meme.output.txt"
    MEME_PID=$(awk '/pid:/{print $2}' <<< $COMMAND_OUTPUT)
}

memtierd-meme-stop() {
    vm-command "killall -KILL meme"
}
