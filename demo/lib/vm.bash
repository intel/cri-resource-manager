source "$(dirname "${BASH_SOURCE[0]}")/command.bash"

VM_PROMPT=${VM_PROMPT-"\e[38;5;11mroot@vm>\e[0m "}
VM_SSH_USER=ubuntu
VM_IMAGE_URL=https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img
VM_IMAGE=$(basename "$VM_IMAGE_URL")

VM_GOVM_COMPOSE_TEMPLATE="vms:
  - name: \${VM_NAME}
    image: \${VM_IMAGE}
    cloud: true
    ContainerEnvVars:
      - KVM_CPU_OPTS=\$(echo "\${KVM_CPU_OPTS}")
      - EXTRA_QEMU_OPTS=\$(echo "\${EXTRA_QEMU_OPTS}")
      - USE_NET_BRIDGES=${USE_NET_BRIDGES-0}
"

vm-check-env() {
    type -p govm >& /dev/null || {
        echo "ERROR:"
        echo "ERROR: environment check failed:"
        echo "ERROR:   govm binary not found."
        echo "ERROR:"
        echo "ERROR: You can install it using the following commands:"
        echo "ERROR:"
        echo "ERROR:     git clone https://github.com/govm-project/govm"
        echo "ERROR:     cd govm"
        echo "ERROR:     go build -o govm"
        echo "ERROR:     cp -v govm \$GOPATH/bin"
        echo "ERROR:     docker build . -t govm/govm:latest"
        echo "ERROR:     cd .."
        echo "ERROR:"
        return 1
    }
    docker inspect govm/govm >& /dev/null || {
        echo "ERROR:"
        echo "ERROR: environment check failed:"
        echo "ERROR:   govm/govm docker image not present (but govm needs it)."
        echo "ERROR:"
        echo "ERROR: You can install it using the following commands:"
        echo "ERROR:"
        echo "ERROR:     git clone https://github.com/govm-project/govm"
        echo "ERROR:     cd govm"
        echo "ERROR:     docker build . -t govm/govm:latest"
        echo "ERROR:     cd .."
        echo "ERROR:"
        return 1
    }
    if [ ! -e ${HOME}/.ssh/id_rsa.pub ]; then
        echo "ERROR:"
        echo "ERROR: environment check failed:"
        echo "ERROR:   id_rsa.pub SSH public key not found (but govm needs it)."
        echo "ERROR:"
        echo "ERROR: You can generate it using the following command:"
        echo "ERROR:"
        echo "ERROR:     ssh-keygen"
        echo "ERROR:"
        return 1
    fi
}

vm-check-binary-cri-resmgr() {
    # Check running cri-resmgr version, print warning if it is not
    # the latest local build.
    if [ -f "$BIN_DIR/cri-resmgr" ] && [ "$(vm-command-q 'md5sum < /proc/$(pidof cri-resmgr)/exe')" != "$(md5sum < "$BIN_DIR/cri-resmgr")" ]; then
        echo "WARNING:"
        echo "WARNING: Running cri-resmgr binary is different from"
        echo "WARNING: $BIN_DIR/cri-resmgr"
        echo "WARNING: Consider restarting with \"reinstall_cri_resmgr=1\" or"
        echo "WARNING: run.sh> uninstall cri-resmgr; install cri-resmgr; launch cri-resmgr"
        echo "WARNING:"
        sleep ${warning_delay}
        return 1
    fi
    return 0
}

vm-command() { # script API
    # Usage: vm-command COMMAND
    #
    # Execute COMMAND on virtual machine as root.
    # Returns the exit status of the execution.
    # Environment variable COMMAND_OUTPUT contains what COMMAND printed
    # in standard output and error.
    #
    # Examples:
    #   vm-command "kubectl get pods"
    #   vm-command "whoami | grep myuser" || command-error "user is not myuser"
    command-start "vm" "$VM_PROMPT" "$1"
    if [ "$2" == "bg" ]; then
        ( ssh -o StrictHostKeyChecking=No ${VM_SSH_USER}@${VM_IP} sudo bash <<<"$COMMAND" 2>&1 | command-handle-output ;
          command-end ${PIPESTATUS[0]}
        ) &
        command-runs-in-bg
    else
        ssh -o StrictHostKeyChecking=No ${VM_SSH_USER}@${VM_IP} sudo bash <<<"$COMMAND" 2>&1 | command-handle-output ;
        command-end ${PIPESTATUS[0]}
    fi
    return $COMMAND_STATUS
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

vm-set-kernel-cmdline() { # script API
    # Usage: vm-set-kernel-cmdline E2E-DEFAULTS
    #
    # Adds/replaces E2E-DEFAULTS to kernel command line"
    #
    # Example:
    #   vm-set-kernel-cmdline nr_cpus=4
    #   vm-reboot
    #   vm-command "cat /proc/cmdline"
    #   launch cri-resmgr
    local e2e_defaults="$1"
    vm-command "echo 'GRUB_CMDLINE_LINUX_DEFAULT=\"\${GRUB_CMDLINE_LINUX_DEFAULT} ${e2e_defaults}\"' > /etc/default/grub.d/60-e2e-defaults.cfg" || {
        command-error "writing new command line parameters failed"
    }
    vm-command "update-grub" || {
        command-error "updating grub failed"
    }
}

vm-reboot() { # script API
    # Usage: vm-reboot
    #
    # Reboots the virtual machine and waits that the ssh server starts
    # responding again.
    vm-command "reboot"
    sleep 5
    host-wait-vm-ssh-server
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

vm-install-cri-resmgr() {
    prefix=/usr/local
    if [ "$binsrc" == "github" ]; then
        vm-command "apt install -y golang make"
        vm-command "go get -d -v github.com/intel/cri-resource-manager"
        CRI_RESMGR_SOURCE_DIR=$(awk '/package.*cri-resource-manager/{print $NF}' <<< "$COMMAND_OUTPUT")
        vm-command "cd $CRI_RESMGR_SOURCE_DIR && make install && cd -"
    elif [[ "$binsrc" == "packages/debian"* ]] || [[ "$binsrc" == "packages/ubuntu"* ]]; then
        vm-command "rm -f *.deb"
        local deb_count
        deb_count=$(ls "$HOST_PROJECT_DIR/$binsrc"/cri-resource-manager_*.deb | wc -l)
        if [ "$deb_count" == "0" ]; then
            error "installing from $binsrc failed: cannot find cri-resource-manager_*deb from $HOST_PROJECT_DIR/$binsrc"
        elif [[ "$deb_count" > "1" ]]; then
            error "installing from $binsrc failed: expected exactly one cri-resource-manager_*.deb in $HOST_PROJECT_DIR/$binsrc, found $deb_count alternatives."
        fi
        host-command "scp $HOST_PROJECT_DIR/$binsrc/*.deb $VM_SSH_USER@$VM_IP:" || {
            command-error "copying *.deb to vm failed, run \"make cross-deb\" first"
        }
        vm-command "dpkg -i *.deb" || {
            command-error "installing packages failed"
        }
        vm-command "systemd daemon-reload"
    elif [ -z "$binsrc" ] || [ "$binsrc" == "local" ]; then
        local bin_change
        local src_change
        bin_change=$(stat --format "%Z" "$BIN_DIR/cri-resmgr")
        src_change=$(find "$HOST_PROJECT_DIR" -name '*.go' -type f | xargs stat --format "%Z" | sort -n | tail -n 1)
        if [[ "$src_change" > "$bin_change" ]]; then
            echo "WARNING:"
            echo "WARNING: Source files changed - installing possibly outdated binaries from"
            echo "WARNING: $BIN_DIR/"
            echo "WARNING:"
            sleep ${warning_delay}
        fi
        host-command "scp \"$BIN_DIR/cri-resmgr\" \"$BIN_DIR/cri-resmgr-agent\" $VM_SSH_USER@$VM_IP:" || {
            command-error "copying local cri-resmgr to VM failed"
        }
        vm-command "mv cri-resmgr cri-resmgr-agent $prefix/bin" || {
            command-error "installing cri-resmgr to $prefix/bin failed"
        }
    else
        error "vm-install-cri-resmgr: unknown binsrc=\"$binsrc\""
    fi
}

vm-install-containerd() {
    vm-command-q "[ -f /usr/bin/containerd ]" || {
        vm-command "apt update && apt install -y containerd"
        # Set proxy environment to containers managed by containerd, if needed.
        if [ -n "$http_proxy" ] || [ -n "$https_proxy" ] || [ -n "$no_proxy" ]; then
            speed=120 vm-command "mkdir -p /etc/systemd/system/containerd.service.d; (echo '[Service]'; echo 'Environment=HTTP_PROXY=$http_proxy'; echo 'Environment=HTTPS_PROXY=$https_proxy'; echo \"Environment=NO_PROXY=$no_proxy,$VM_IP,10.96.0.0/12,10.217.0.0/16,\$(hostname)\" ) > /etc/systemd/system/containerd.service.d/proxy.conf; systemctl daemon-reload; systemctl restart containerd"
        fi
    }
}

vm-install-containernetworking() {
    vm-command-q "command -v go >/dev/null" || vm-command "apt update && apt install -y golang"
    vm-command "go get -d github.com/containernetworking/plugins"
    CNI_PLUGINS_SOURCE_DIR=$(awk '/package.*plugins/{print $NF}' <<< $COMMAND_OUTPUT)
    [ -n "$CNI_PLUGINS_SOURCE_DIR" ] || {
        command-error "downloading containernetworking plugins failed"
    }
    vm-command "pushd \"$CNI_PLUGINS_SOURCE_DIR\" && ./build_linux.sh && mkdir -p /opt/cni && cp -rv bin /opt/cni && popd" || {
        command-error "building and installing cri-tools failed"
    }
    vm-command 'rm -rf /etc/cni/net.d && mkdir -p /etc/cni/net.d && cat > /etc/cni/net.d/10-bridge.conf <<EOF
{
  "cniVersion": "0.4.0",
  "name": "mynet",
  "type": "bridge",
  "bridge": "cni0",
  "isGateway": true,
  "ipMasq": true,
  "ipam": {
    "type": "host-local",
    "subnet": "10.217.0.0/16",
    "routes": [
      { "dst": "0.0.0.0/0" }
    ]
  }
}
EOF'
    vm-command 'cat > /etc/cni/net.d/20-portmap.conf <<EOF
{
    "cniVersion": "0.4.0",
    "type": "portmap",
    "capabilities": {"portMappings": true},
    "snat": true
}
EOF'
    vm-command 'cat > /etc/cni/net.d/99-loopback.conf <<EOF
{
  "cniVersion": "0.4.0",
  "name": "lo",
  "type": "loopback"
}
EOF'
}

vm-install-k8s() {
    vm-command "apt update && apt install -y apt-transport-https curl"
    speed=60 vm-command "curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -"
    speed=60 vm-command "echo \"deb https://apt.kubernetes.io/ kubernetes-xenial main\" > /etc/apt/sources.list.d/kubernetes.list"
    vm-command "apt update &&  apt install -y kubelet kubeadm kubectl"
}

vm-create-singlenode-cluster-cilium() {
    vm-command "kubeadm init --pod-network-cidr=10.217.0.0/16 --cri-socket /var/run/cri-resmgr/cri-resmgr.sock"
    if ! grep -q "initialized successfully" <<< "$COMMAND_OUTPUT"; then
        command-error "kubeadm init failed"
    fi
    vm-command "mkdir -p \$HOME/.kube"
    vm-command "cp -i /etc/kubernetes/admin.conf \$HOME/.kube/config"
    vm-command "kubectl taint nodes --all node-role.kubernetes.io/master-"
    vm-command "kubectl create -f https://raw.githubusercontent.com/cilium/cilium/v1.6/install/kubernetes/quick-install.yaml"
    if ! vm-command "kubectl rollout status --timeout=360s -n kube-system daemonsets/cilium"; then
        command-error "installing cilium CNI to Kubernetes timed out"
    fi
}

vm-print-usage() {
    echo "- Login VM:     ssh $VM_SSH_USER@$VM_IP"
    echo "- Stop VM:      govm stop $VM_NAME"
    echo "- Delete VM:    govm delete $VM_NAME"
}

vm-check-env || exit 1
