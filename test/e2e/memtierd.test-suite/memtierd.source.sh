MEMTIERD_PORT=${MEMTIERD_PORT:-5555}
MEMTIERD_OUTPUT=memtierd.output.txt

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
    vm-command "nohup sh -c 'socat tcp4-listen:${MEMTIERD_PORT},fork,reuseaddr - | memtierd -config memtierd.yaml -debug' > ${MEMTIERD_OUTPUT} 2>&1 & sleep 2; cat ${MEMTIERD_OUTPUT}"
}

memtierd-stop() {
    memtierd-command "q"
    sleep 1
    vm-command "killall -KILL memtierd"
}

memtierd-command() {
    vm-command "offset=\$(wc -l ${MEMTIERD_OUTPUT} | awk '{print \$1+1}'); echo '$1' | socat - tcp4:localhost:${MEMTIERD_PORT}; sleep 1; tail -n+\${offset} ${MEMTIERD_OUTPUT}"
}

memtierd-meme-start() {
    vm-command "nohup meme -bs ${MEME_BS:-1G} -brc ${MEME_BRC:-0} -bwc ${MEME_BWC:-0} -bws ${MEME_BWS:-0} -bwo ${MEME_BWO:-0} -ttl ${MEME_TTL:-1h} < /dev/null > meme.output.txt 2>&1 & sleep 2; cat meme.output.txt"
    MEME_PID=$(awk '/pid:/{print $2}' <<< $COMMAND_OUTPUT)
}

memtierd-meme-stop() {
    vm-command "killall -KILL meme"
}
