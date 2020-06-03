VM_PROMPT=${VM_PROMPT-"\e[38;5;11mroot@vm>\e[0m "}
VM_SSH_USER=ubuntu
VM_IMAGE_URL=https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img
VM_IMAGE=$(basename "$VM_IMAGE_URL")

vm-command() {
    speed=${speed-10}
    if [ -n "$outcolor" ]; then
        OUTSTART="\e[38;5;${outcolor}m"
        OUTEND="\e[0m"
    else
        OUTSTART=""
        OUTEND=""
    fi
    if [ -n "$PV" ]; then
        echo -e -n "${VM_PROMPT}"
        echo "$1" | $PV $speed
    fi
    ssh -o StrictHostKeyChecking=No ${VM_SSH_USER}@${VM_IP} sudo bash <<<"$1" 2>&1 | tee command.output | ( echo -e -n "$OUTSTART"; cat; echo -e -n "$OUTEND" )
    command_status=${PIPESTATUS[0]}
    if [ -n "$PV" ]; then
        echo | $PV $speed
    fi
    return $command_status
}

vm-command-q() {
    ssh -o StrictHostKeyChecking=No ${VM_SSH_USER}@${VM_IP} sudo bash <<<"$1"
}

vm-wait-process() {
    # parameters: "process" and "timeout" (optional, default 30 seconds)
    process=$1
    timeout=${2-30}
    if ! vm-command-q "retry=$timeout; until ps -A | grep -q $process; do retry=\$(( \$retry - 1 )); [ \"\$retry\" == \"0\" ] && exit 1; sleep 1; done"; then
        error "waiting for process \"$process\" timed out"
    fi
}

vm-networking() {
    vm-command-q "grep -q 1 /proc/sys/net/ipv4/ip_forward" || vm-command "sysctl -w net.ipv4.ip_forward=1"
    vm-command-q "grep -q ^net.ipv4.ip_forward=1 /etc/sysctl.conf" || vm-command "sed -i 's/#net.ipv4.ip_forward=1/net.ipv4.ip_forward=1/g' /etc/sysctl.conf"
    vm-command-q "grep -q 1 /proc/sys/net/bridge/bridge-nf-call-iptables 2>/dev/null" || {
        vm-command "modprobe br_netfilter"
        vm-command "echo br_netfilter > /etc/modules-load.d/br_netfilter.conf"
    }
    vm-command-q "grep -q \$(hostname) /etc/hosts" || vm-command "echo \"$VM_IP \$(hostname)\" >/etc/hosts"
    if [ -n "$http_proxy" ] || [ -n "$https_proxy" ] || [ -n "$no_proxy" ]; then
        vm-command-q "grep -q no_proxy /etc/environment" || vm-command "(echo http_proxy=$http_proxy; echo https_proxy=$https_proxy; echo no_proxy=$no_proxy,$VM_IP,10.96.0.0/12,10.217.0.0/16,\$(hostname) ) >> /etc/environment"
    fi
}
