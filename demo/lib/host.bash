source "$(dirname "${BASH_SOURCE[0]}")/command.bash"

HOST_PROMPT=${HOST_PROMPT-"\e[38;5;11mhost>\e[0m "}
GOVM=${GOVM-govm}

host-command() {
    command-start "host" "$HOST_PROMPT" "$1"
    bash -c "$COMMAND" 2>&1 | command-handle-output
    command-end ${PIPESTATUS[0]}
    return $COMMAND_STATUS
}

host-require-govm() {
    command -v "$GOVM" >/dev/null || error "cannot run govm \"$GOVM\". Check PATH or set GOVM=/path/to/govm."
}

host-create-vm() {
    VM_NAME=$1
    if [ -z "$VM_NAME" ]; then
        error "cannot create VM: missing name"
    fi
    VM_OPT_CPUS=${VM_OPT_CPUS-4}
    host-require-govm
    # If VM does not exist, create it from scrach
    ${GOVM} ls | grep -q "$VM_NAME" || {
        [ -f "$VM_IMAGE" ] || {
            host-command "wget -q -O \"$VM_IMAGE\" \"$VM_IMAGE_URL\"" || error "failed to download VM image from $VM_IMAGE_URL"
        }
        host-command "${GOVM} create --ram 8192 --cpus ${VM_OPT_CPUS} --image \"$VM_IMAGE\" --name \"$VM_NAME\" --cloud"
    }

    VM_IP=$(${GOVM} ls | awk "/$VM_NAME/{print \$4}")
    while [ "x$VM_IP" == "x" ]; do
        host-command "${GOVM} start \"$VM_NAME\""
        sleep 5
        VM_IP=$(${GOVM} ls | awk "/$VM_NAME/{print \$4}")
    done

    ssh-keygen -f "$HOME/.ssh/known_hosts" -R "$VM_IP" >/dev/null 2>&1
    retries=60
    retries_left=$retries
    while ! ssh -o ConnectTimeout=2 -o StrictHostKeyChecking=No ${VM_SSH_USER}@${VM_IP} true 2>/dev/null; do
        if [ "$retries" == "$retries_left" ]; then
            echo -n "Waiting for VM SSH server to respond..."
        fi
        sleep 2
        echo -n "."
        retries_left=$(( $retries_left - 1 ))
        if [ "$retries_left" == "0" ]; then
            error "timeout"
        fi
    done
    [ "$retries" == "$retries_left" ] || echo ""
}

host-stop-vm() {
    VM_NAME=$1
    host-require-govm
    host-command "${GOVM} stop $VM_NAME" || {
        command-error "stopping govm \"$VM_NAME\" failed"
    }
}

host-delete-vm() {
    VM_NAME=$1
    host-require-govm
    host-command "${GOVM} delete $VM_NAME" || {
        command-error "deleting govm \"$VM_NAME\" failed"
    }
}
