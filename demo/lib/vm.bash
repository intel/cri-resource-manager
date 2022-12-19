# shellcheck disable=SC1091
# shellcheck source=command.bash
source "$(dirname "${BASH_SOURCE[0]}")/command.bash"
# shellcheck disable=SC1091
# shellcheck source=distro.bash
source "$(dirname "${BASH_SOURCE[0]}")/distro.bash"

VM_PROMPT=${VM_PROMPT-"\e[38;5;11mroot@vm>\e[0m "}

vm-compose-govm-template() {
    (echo "
vms:
  - name: ${VM_NAME}
    image: ${VM_IMAGE}
    cloud: true
    ContainerEnvVars:
      - KVM_CPU_OPTS=${VM_QEMU_CPUMEM:=-machine pc -smp cpus=4 -m 8G}
      - EXTRA_QEMU_OPTS=-monitor unix:/data/monitor,server,nowait ${VM_QEMU_EXTRA}
      - USE_NET_BRIDGES=${USE_NET_BRIDGES:-0}
$(for govm_env in $(distro-govm-env); do echo "
      - ${govm_env}"; done)
    user-data: |
      #!/bin/bash
      set -e
"
     (if [ -n "$VM_EXTRA_BOOTSTRAP_COMMANDS" ]; then
          # shellcheck disable=SC2001
          sed 's/^/      /g' <<< "${VM_EXTRA_BOOTSTRAP_COMMANDS}"
     fi
      # shellcheck disable=SC2001
      sed 's/^/      /g' <<< "$(distro-bootstrap-commands)")) |
        grep -E -v '^ *$'
}

vm-bootstrap() {
    distro-bootstrap-commands | vm-pipe-to-file "./e2e-bootstrap.sh"
    vm-command "sh ./e2e-bootstrap.sh"
    host-wait-vm-ssh-server --timeout 600
}

vm-image-url() {
    distro-image-url
}

vm-ssh-user() {
    if [ -n "$VM_SSH_USER" ]; then
        echo "$VM_SSH_USER"
    else
        distro-ssh-user
    fi
}


vm-is-govm() { # script API
    local name="${1:-$VM_NAME}"
    # Usage: vm-is-govm [name]
    #
    # Check if the given name (or $VM_NAME if omitted) corresponds to
    # a govm-managed virtual machine. Returns 0 if it does. Returns 1
    # if it does not. Returns 2 if govm is not installed.

    if ! type -f govm >& /dev/null; then
        return 2
    fi
    if [ -z "$name" ]; then
        return 1
    fi

    if govm ls | cut -d ' ' -f 2 | grep -q "^$name$"; then
       return 0
    fi

    return 1
}

vm-check-env() {
    # If VM IP address is already defined, govm is not needed.
    if [ -n "$VM_IP" ]; then
        if [ "x$(vm-command-q "whoami")" != "xroot" ]; then
            echo "ERROR:"
            echo "ERROR: environment check failed:"
            echo "ERROR:   cannot run commands (with sudo) when connecting"
            echo "ERROR:   $SSH $VM_SSH_USER@$VM_IP"
            echo "ERROR:"
            return 1
        fi
        return 0
    fi
    # Check that VM created/managed with govm in this environment.
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
    if [ ! -e "$SSH_KEY".pub ]; then
        echo "ERROR:"
        echo "ERROR: environment check failed:"
        echo "ERROR:   $SSH_KEY.pub SSH public key not found (but govm needs it)."
        echo "ERROR:"
        echo "ERROR: You can generate it using the following command:"
        echo "ERROR:"
        echo "ERROR:     ssh-keygen"
        echo "ERROR:"
        return 1
    fi
    if [ -n "$SSH_AUTH_SOCK" ] && [ -e "$SSH_AUTH_SOCK" ]; then
        if ! ssh-add -l | grep -q "$(ssh-keygen -l -f "$SSH_KEY" < /dev/null 2>/dev/null | awk '{print $2}')"; then
            if ! ssh-add "$SSH_KEY" < /dev/null; then
                echo "ERROR:"
                echo "ERROR: environment setup failed:"
                echo "ERROR:   Failed to load $SSH_KEY SSH key to agent."
                echo "ERROR:"
                echo "ERROR: Please make sure an SSH agent is running, then"
                echo "ERROR: try loading the key using the following command:"
                echo "ERROR:"
                echo "ERROR:     ssh-add $SSH_KEY"
                echo "ERROR:"
                return 1
            fi
        fi
    else
        if host-is-encrypted-ssh-key "$SSH_KEY"; then
            echo "ERROR:"
            echo "ERROR: environment setup failed:"
            echo "ERROR:   $SSH_KEY SSH key is encrypted, but agent is not running."
            echo "ERROR:"
            echo "ERROR: Please make sure an SSH agent is running, then"
            echo "ERROR: try loading the key using the following command:"
            echo "ERROR:"
            echo "ERROR:     ssh-add $SSH_KEY"
            echo "ERROR:"
            return 1
        fi
    fi
}

vm-check-running-binary() {
    local bin_file="$1"
    local bin_name
    bin_name="$(basename "$bin_file")"
    pid_of_bin="$(vm-command-q "pidof $bin_name")"
    if [ -f "$bin_file" ] && [ -n "$pid_of_bin" ] && [ "$(vm-command-q "md5sum < /proc/$pid_of_bin/exe")" != "$(md5sum < "$bin_file")" ]; then
        echo "WARNING:"
        echo "WARNING: Running $bin_name binary is different from"
        echo "WARNING: $bin_file"
        echo "WARNING: Consider restarting with reinstall_${bin_name//-/_}=1."
        echo "WARNING:"
        sleep "${warning_delay:-0}"
        return 1
    fi
    return 0
}

vm-check-source-files-changed() {
    local bin_change
    local src_change
    local src_dir="$1"
    local bin_file="$2"
    bin_change=$(stat --format "%Z" "$bin_file")
    src_change=$(find "$src_dir" -name '*.go' -type f -print0 | xargs -0 stat --format "%Z" | sort -n | tail -n 1)
    if [[ "$src_change" > "$bin_change" ]]; then
        echo "WARNING:"
        echo "WARNING: Source files changed, outdated binaries in"
        echo "WARNING: $(dirname "$bin_file")/"
        echo "WARNING:"
        sleep "${warning_delay:-0}"
    fi
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
        ( $SSH "${VM_SSH_USER}@${VM_IP}" sudo bash -l <<<"$COMMAND" 2>&1 | command-handle-output ;
          command-end "${PIPESTATUS[0]}"
        ) &
        command-runs-in-bg
    else
        $SSH "${VM_SSH_USER}@${VM_IP}" sudo bash -l <<<"$COMMAND" 2>&1 | command-handle-output ;
        command-end "${PIPESTATUS[0]}"
    fi
    return "$COMMAND_STATUS"
}

vm-command-q() {
    $SSH "${VM_SSH_USER}@${VM_IP}" sudo bash -l <<<"$1"
}

vm-ssh-user-ip() {
    # Usage: vm-ssh-user-ip NODE
    #
    # Print canonical USER@HOST for NODE. NODE can be a govm vm name
    # or already of the form: USER@HOST.
    local NODE="$1"
    local node_ssh_user=""
    local node_ssh_ip=""
    if [[ "$NODE" == *"@"* ]]; then
        node_ssh_ip=${NODE/*@}
        node_ssh_user=${NODE%@*}
    else
        node_ssh_ip=$(${GOVM} ls | awk "/$NODE/{print \$4}")
        node_ssh_user=$( host-get-vm-config $NODE && echo $VM_SSH_USER )
    fi
    if [ -z "$node_ssh_ip" ]; then
        error "cannot find IP address for NODE=$NODE"
    fi
    if [ -z "$node_ssh_user" ]; then
        error "cannot find ssh user for NODE=$NODE"
    fi
    echo "${node_ssh_user}@${node_ssh_ip}"
}

vm-join() {
    # Usage: vm-join MASTER_NODE
    #
    # Join vm to the cluster whose master node is MASTER_NODE."
    # MASTER_NODE is a name of a govm virtual machine, or
    # "USER@HOST" that can be logged into using ssh.
    local MASTER_NODE="$1"
    local master_user_ip
    local k8s_join_cmd
    k8s_join_cmd="$(vm-join-cmd "$MASTER_NODE")"
    vm-command "$k8s_join_cmd" || {
        command-error "joining to the cluster master ($MASTER_NODE) failed"
    }
    # Enable using kubectl on the worker vm by
    # copying k8s admin configuration on it.
    master_user_ip="$(vm-ssh-user-ip $MASTER_NODE)"
    ssh "$master_user_ip" "sudo cat /etc/kubernetes/admin.conf" | vm-pipe-to-file "/root/.kube/config"
}

vm-join-cmd() {
    # Usage: vm-join-cmd MASTER_NODE
    #
    # Print a join command to join VM to existing cluster MASTER_NODE.
    # MASTER_NODE is a name of a govm virtual machine (exists in "govm ls")
    # or USERNAME@IP.
    local MASTER_NODE="$1"
    local master_user_ip
    local k8s_join_cmd=""
    master_user_ip="$(vm-ssh-user-ip $MASTER_NODE)"
    local ssh_get_join_cmd="ssh $master_user_ip sudo kubeadm token create --print-join-command"
    k8s_join_cmd="$( $ssh_get_join_cmd )"
    if [[ "$k8s_join_cmd" != *" join "* ]]; then
        error "failed to get kubeadm join command: $k8s_join_cmd"
    fi
    echo $k8s_join_cmd
}

vm-mem-hotplug() { # script API
    # Usage: vm-mem-hotplug MEMORY
    #
    # Hotplug currently unplugged MEMORY to VM.
    # Find unplugged memory with "vm-mem-hw | grep unplugged".
    #
    # Examples:
    #   vm-mem-hotplug mem2
    local memmatch memline memid memdimm memnode memdriver
    memmatch=$1
    if [ -z "$memmatch" ]; then
        error "missing MEMORY"
        return 1
    fi
    memline="$(vm-mem-hw | grep unplugged | grep "$memmatch")"
    if [ -z "$memline" ]; then
        error "unplugged memory matching '$memmatch' not found"
        return 1
    fi
    memid="$(awk '{print $1}' <<< "$memline")"
    memid=${memid#mem}
    memid=${memid%[: ]*}
    memdimm="$(awk '{print $2}' <<< "$memline")"
    memnode="$(awk '{print $4}' <<< "$memline")"
    memnode=${memnode#node}
    if [ "$memdimm" == "nvdimm" ]; then
        memdriver="nvdimm"
    else
        memdriver="pc-dimm"
    fi
    vm-monitor "device_add ${memdriver},id=${memdimm}${memid},memdev=mem${memdimm}_${memid}_node_${memnode},node=${memnode}"
}

vm-mem-hotremove() { # script API
    # Usage: vm-mem-hotremove MEMORY
    #
    # Hotremove currently plugged MEMORY from VM.
    # Find plugged memory with "vm-mem-hw | grep ' plugged'".
    #
    # Examples:
    #   vm-mem-hotremove mem2
    local memmatch memline memid memdimm memnode memdriver
    memmatch=$1
    if [ -z "$memmatch" ]; then
        error "missing MEMORY"
        return 1
    fi
    memline="$(vm-mem-hw | grep \ plugged | grep "$memmatch")"
    if [ -z "$memline" ]; then
        error "plugged memory matching '$memmatch' not found"
        return 1
    fi
    memid="$(awk '{print $1}' <<< "$memline")"
    memid=${memid#mem}
    memid=${memid%[: ]*}
    memdimm="$(awk '{print $2}' <<< "$memline")"
    vm-monitor "device_del ${memdimm}${memid}"
}

vm-mem-hw() { # script API
    # Usage: vm-mem-hw
    #
    # List VM memory hardware with current status.
    # See also: vm-mem-hotplug, vm-mem-hotremove
    vm-monitor "$(echo info memdev; echo info memory-devices)" | awk '
      /memdev: /{
          split($2,a,"_");
          state[a[2]]="plugged  ";
      }
      /memory backend: membuiltin/{
          split($3,a,"_"); backend=1;
          type[a[2]]="ram    "; state[a[2]]="builtin  "; node[a[2]]=a[4];
      }
      /memory backend: memnvbuiltin/{
          split($3,a,"_"); backend=1;
          type[a[2]]="nvram  "; state[a[2]]="builtin  "; node[a[2]]=a[4];
      }
      /memory backend: memnvdimm/{
          split($3,a,"_"); backend=1;
          type[a[2]]="nvdimm "; state[a[2]]="unplugged"; node[a[2]]=a[4];
      }
      /memory backend: memdimm/{
          split($3,a,"_"); backend=1;
          type[a[2]]="dimm   "; state[a[2]]="unplugged"; node[a[2]]=a[4];
      }
      /size: /{sz=$2/1024/1024; if (backend==1) {size[a[2]]=sz;backend=0;}}
      END{
          for (m in node) print "mem"m": "type[m]" "state[m]" node"node[m]" size="size[m]"M";
      }'
}

vm-monitor() { # script API
    # Usage: vm-monitor COMMAND
    #
    # Execute COMMAND on Qemu monitor.
    #
    # Example: VM monitor help:
    #  vm-monitor "help" | less
    #
    # Example: print memdev objects and plugged in memory devices:
    #  vm-monitor "info memdev"
    #  vm-monitor "info memory-devices"
    #
    # Example: hot plug a NVDIMM to NUMA node 1 when launched with topology
    # topology='[{"cores":2,"mem":"2G"},{"nvmem":"4G","dimm":"unplugged"}]':
    #   vm-monitor "device_add pc-dimm,id=nvdimm0,memdev=nvmem0,node=1"
    [ -n "$VM_MONITOR" ] ||
        error "VM is not running"
    eval "$VM_MONITOR" <<< "$1" | sed 's/\r//g'
    if [ "${PIPESTATUS[0]}" != "0" ]; then
        error "sending command to Qemu monitor failed"
    fi
    echo ""
}

vm-wait-process() { # script API
    # Usage: vm-wait-process [--timeout TIMEOUT] [--pidfile PIDFILE] PROCESS
    #
    # Wait for a PROCESS (string) to appear in process list (pidof output).
    # If pidfile parameter is given, we also check that the process has that file open.
    # The default TIMEOUT is 30 seconds.
    local process timeout pidfile invalid
    timeout=30
    while [ "${1#-}" != "$1" ] && [ -n "$1" ]; do
        case "$1" in
            --timeout)
                timeout="$2"
                shift 2
                ;;
            --pidfile)
                pidfile="$2"
                shift 2
                ;;
            *)
                invalid="${invalid}${invalid:+,}\"$1\""
                shift
                ;;
        esac
    done
    if [ -n "$invalid" ]; then
        error "invalid options: $invalid"
        return 1
    fi
    process="$1"
    vm-run-until --timeout "$timeout" "pidof \"$process\" > /dev/null" || error "timeout while waiting $process"

    # As we first wait for the process, and then wait for the pidfile (if enabled)
    # we might wait longer than expected. Accept that anomaly atm.
    if [ ! -z "$pidfile" ]; then
	vm-run-until --timeout $timeout "[ ! -z \"\$(fuser $pidfile 2>/dev/null)\" ]" || error "timeout while waiting $pidfile"
	vm-run-until --timeout $timeout "[ \$(fuser $pidfile 2>/dev/null) -eq \$(pidof $process) ]" || error "timeout while waiting $process and $pidfile"
    fi
}

vm-run-until() { # script API
    # Usage: vm-run-until [--timeout TIMEOUT] CMD
    #
    # Keep running CMD (string) until it exits successfully.
    # The default TIMEOUT is 30 seconds.
    local cmd timeout invalid
    timeout=30
    while [ "${1#-}" != "$1" ] && [ -n "$1" ]; do
        case "$1" in
            --timeout)
                timeout="$2"
                shift; shift
                ;;
            *)
                invalid="${invalid}${invalid:+,}\"$1\""
                shift
                ;;
        esac
    done
    if [ -n "$invalid" ]; then
        error "invalid options: $invalid"
        return 1
    fi
    cmd="$1"
    if ! vm-command-q "retry=$timeout; until $cmd; do retry=\$(( \$retry - 1 )); [ \"\$retry\" == \"0\" ] && exit 1; sleep 1; done"; then
        error "waiting for command \"$cmd\" to exit successfully timed out after $timeout s"
    fi
}

vm-write-file() {
    local vm_path_file="$1"
    local file_content_b64
    file_content_b64="$(base64 <<<"$2")"
    vm-command-q "mkdir -p $(dirname "$vm_path_file"); echo -n \"$file_content_b64\" | base64 -d > \"$vm_path_file\""
}

vm-put-file() { # script API
    # Usage: vm-put-file [--cleanup] [--append] SRC-HOST-FILE DST-VM-FILE
    #
    # Copy SRC-HOST-FILE to DST-VM-FILE on the VM, removing
    # SRC-HOST-FILE if called with the --cleanup flag, and
    # appending instead of copying if the --append flag is
    # specified.
    #
    # Example:
    #   src=$(mktemp) && \
    #       echo 'Ahoy, Matey...' > $src && \
    #       vm-put-file --cleanup $src /etc/motd
    local cleanup append invalid
    while [ "${1#-}" != "$1" ] && [ -n "$1" ]; do
        case "$1" in
            --cleanup)
                cleanup=1
                shift
                ;;
            --append)
                append=1
                shift
                ;;
            *)
                invalid="${invalid}${invalid:+,}\"$1\""
                shift
                ;;
        esac
    done
    if [ -n "$cleanup" ] && [ -n "$1" ]; then
        # shellcheck disable=SC2064
        trap "rm -f \"$1\"" RETURN EXIT
    fi
    if [ -n "$invalid" ]; then
        error "invalid options: $invalid"
        return 1
    fi
    [ "$(dirname "$2")" == "." ] || vm-command-q "[ -d \"$(dirname "$2")\" ]" || vm-command "mkdir -p \"$(dirname "$2")\"" ||
        command-error "cannot create vm-put-file destination directory to VM"
    host-command "$SCP \"$1\" ${VM_SSH_USER}@${VM_IP}:\"vm-put-file.${1##*/}\"" ||
        command-error "failed to copy file to VM"
    if [ -z "$append" ]; then
        vm-command "mv \"vm-put-file.${1##*/}\" \"$2\"" ||
            command-error "failed to rename file"
    else
        vm-command "touch \"$2\" && cat \"vm-put-file.${1##*/}\" >> \"$2\" && rm -f \"vm-put-file.${1##*/}\"" ||
            command-error "failed to append file"
    fi
}

vm-put-pkg() { # script API
    # Usage: vm-put-pkg [--force] HOST-FILE...
    #
    # Copies HOST-FILEs from host to vm and installs them.
    #
    # Examples:
    #   vm-put-pkg /tmp/kernel.rpm /tmp/myutil.rpm
    local host_pkg
    local vm_pkgs=""
    local force=""
    if [ "$1" == "--force" ]; then
        force="--force "
        shift
    fi
    for host_pkg in "$@"; do
        local vm_pkg="pkgs/$(basename "$host_pkg")"
        vm-command-q "mkdir -p $(dirname "$vm_pkg")"
        vm-put-file "$host_pkg" "$vm_pkg"
        vm_pkgs="$vm_pkgs $vm_pkg"
    done
    distro-install-pkg-local $force "$vm_pkgs"
}

vm-put-docker-image() { # script API
    # Usage: vm-put-docker-image IMAGE
    #
    # Exports IMAGE from docker images on the host, and
    # imports it in the "k8s.io" namespace (visible
    # for kubernetes containers) on the vm.
    #
    # Works with containerd only.
    #
    # Examples:
    #   vm-put-docker-image busybox:latest
    local image_name="$1"
    local image_file_on_vm="images/${image_name//:/__}"
    vm-command-q "mkdir -p $(dirname "$image_file_on_vm")"
    docker save "$image_name" | vm-pipe-to-file "$image_file_on_vm" ||
        error "failed to save and pipe image '$image_name'"
    vm-cri-import-image "$image_name" "$image_file_on_vm"
}

vm-pipe-to-file() { # script API
    # Usage: vm-pipe-to-file [--append] DST-VM-FILE
    #
    # Reads stdin and writes the content to DST-VM-FILE, creating any
    # intermediate directories necessary.
    #
    # Example:
    #   echo 'Ahoy, Matey...' | vm-pipe-to-file /etc/motd
    local tmp append
    tmp="$(mktemp vm-pipe-to-file.XXXXXX)"
    if [ "$1" = "--append" ]; then
        append="--append"
        shift
    fi
    cat > "$tmp"
    vm-put-file --cleanup $append "$tmp" "$1"
}

vm-sed-file() { # script API
    # Usage: vm-sed-file PATH-IN-VM SED-EXTENDED-REGEXP-COMMANDS
    #
    # Edits the given file in place with the given extended regexp
    # sed commands.
    #
    # Example:
    #   vm-sed-file /etc/motd 's/Matey/Guybrush Threepwood/'
    local file="$1" cmd
    shift
    for cmd in "$@"; do
        vm-command "sed -E -i \"$cmd\" $file" ||
            command-error "failed to edit $file with sed"
    done
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
    distro-set-kernel-cmdline "$@"
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

vm-force-restart() { # script API
    # Usage: vm-force-restart
    #
    # Give the virtual machine a chance to shut itself down, then
    # forcibly restart it using govm stop/start. Wait for the ssh
    # server to start responding again after restarting. If VM_NAME
    # is not set assume the target machine to not be govm-managed
    # and fall back to vm-reboot instead.
    if vm-is-govm; then
        vm-command "shutdown -h now"
        sleep 10
        vm-monitor system_reset
        host-wait-vm-ssh-server
    else
      vm-reboot
    fi
}

vm-setup-proxies() {
    distro-setup-proxies
}

vm-networking() {
    vm-command-q "touch /etc/hosts; grep -q \$(hostname) /etc/hosts" || {
        vm-command "echo \"$VM_IP \$(hostname)\" >>/etc/hosts"
    }

    vm-setup-proxies
}

vm-install-cri-resmgr() {
    prefix=/usr/local
    # shellcheck disable=SC2154
    if [ "$binsrc" == "github" ]; then
        vm-install-golang
        vm-install-pkg make
        vm-command "go get -d -v github.com/intel/cri-resource-manager"
        CRI_RESMGR_SOURCE_DIR=$(awk '/package.*cri-resource-manager/{print $NF}' <<< "$COMMAND_OUTPUT")
        vm-command "cd $CRI_RESMGR_SOURCE_DIR && make install && cd -"
    elif [ "${binsrc#packages/}" != "$binsrc" ]; then
        suf=$(vm-pkg-type)
        vm-command "rm -f *.$suf"
        local pkg_count
        # shellcheck disable=SC2010,SC2126
        pkg_count="$(ls "$HOST_PROJECT_DIR/$binsrc"/cri-resource-manager*."$suf" | grep -v dbg | wc -l)"
        if [ "$pkg_count" == "0" ]; then
            error "installing from $binsrc failed: cannot find cri-resource-manager_*.$suf from $HOST_PROJECT_DIR/$binsrc"
        elif [[ "$pkg_count" -gt 1 ]]; then
            error "installing from $binsrc failed: expected exactly one cri-resource-manager*.$suf in $HOST_PROJECT_DIR/$binsrc, found $pkg_count alternatives."
        fi
        host-command "$SCP $HOST_PROJECT_DIR/$binsrc/*.$suf $VM_SSH_USER@$VM_IP:/tmp" || {
            command-error "copying *.$suf to vm failed, run \"make cross-$suf\" first"
        }
        vm-install-pkg "/tmp/cri-resource-manager*.$suf" || {
            command-error "installing packages failed"
        }
        vm-command "systemctl daemon-reload"
    elif [ -z "$binsrc" ] || [ "$binsrc" == "local" ]; then
        vm-put-file "$BIN_DIR/cri-resmgr" "$prefix/bin/cri-resmgr"
        vm-put-file "$BIN_DIR/cri-resmgr-agent" "$prefix/bin/cri-resmgr-agent"
        sed -E -e "s:__DEFAULTDIR__:$(distro-env-file-dir):g" \
            -E -e "s:__BINDIR__:$prefix/bin:g" < "$HOST_PROJECT_DIR/cmd/cri-resmgr/cri-resource-manager.service.in" |
            vm-pipe-to-file /usr/lib/systemd/system/cri-resource-manager.service
        cat <<EOF |
CONFIG_OPTIONS="--fallback-config /etc/cri-resmgr/fallback.cfg -relay-socket ${cri_resmgr_sock} -runtime-socket ${cri_sock} -image-socket ${cri_sock}"
EOF
        vm-pipe-to-file "$(distro-env-file-dir)/cri-resource-manager"
        vm-put-file "$HOST_PROJECT_DIR/cmd/cri-resmgr/fallback.cfg.sample" "/etc/cri-resmgr/fallback.cfg"
    else
        error "vm-install-cri-resmgr: unknown binsrc=\"$binsrc\""
    fi
}

vm-install-cri-resmgr-agent() {
    prefix=/usr/local
    local bin_change
    local src_change
    bin_change=$(stat --format "%Z" "$BIN_DIR/cri-resmgr-agent")
    src_change=$(find "$HOST_PROJECT_DIR" -name '*.go' -type f -print0 | xargs -0 stat --format "%Z" | sort -n | tail -n 1)
    if [[ "$src_change" > "$bin_change" ]]; then
        echo "WARNING:"
        echo "WARNING: Source files changed - installing possibly outdated binaries from"
        echo "WARNING: $BIN_DIR/"
        echo "WARNING:"
        sleep "${warning_delay:-0}"
    fi
    vm-put-file "$BIN_DIR/cri-resmgr-agent" "$prefix/bin/cri-resmgr-agent"
}

vm-cri-import-image() {
    local image_name="$1"
    local image_tar="$2"
    case "$VM_CRI" in
        containerd)
            vm-command "ctr -n k8s.io images import '$image_tar'" ||
                command-error "failed to import \"$image_tar\" on VM"
            ;;
        *)
            error "vm-cri-import-image unsupported container runtime: \"$VM_CRI\""
    esac
}

vm-install-cri-resmgr-webhook() {
    local service=cri-resmgr-webhook
    local namespace=cri-resmgr
    vm-command-q "\
        kubectl delete secret -n ${namespace} cri-resmgr-webhook-secret 2>/dev/null; \
        kubectl delete csr ${service}.${namespace} 2>/dev/null; \
        kubectl delete -f webhook/mutating-webhook-config.yaml 2>/dev/null; \
        kubectl delete -f webhook/webhook-deployment.yaml 2>/dev/null; \
        "
    local webhook_image_info webhook_image_id webhook_image_repotag webhook_image_tar
    webhook_image_info="$(docker images --filter=reference=cri-resmgr-webhook --format '{{.ID}} {{.Repository}}:{{.Tag}} (created {{.CreatedSince}}, {{.CreatedAt}})' | head -n 1)"
    if [ -z "$webhook_image_info" ]; then
        error "cannot find cri-resmgr-webhook image on host, run \"make images\" and check \"docker images --filter=reference=cri-resmgr-webhook\""
    fi
    echo "installing webhook to VM from image: $webhook_image_info"
    sleep 2
    webhook_image_id="$(awk '{print $1}' <<< "$webhook_image_info")"
    webhook_image_repotag="$(awk '{print $2}' <<< "$webhook_image_info")"
    webhook_image_tar="$(realpath "$OUTPUT_DIR/webhook-image-$webhook_image_id.tar")"
    # It is better to export (save) the image with image_repotag rather than image_id
    # because otherwise manifest.json RepoTags will be null and containerd will
    # remove the image immediately after impoting it as part of garbage collection.
    docker image save "$webhook_image_repotag" > "$webhook_image_tar"
    vm-put-file "$webhook_image_tar" "webhook/$(basename "$webhook_image_tar")" || {
        command-error "copying webhook image to VM failed"
    }
    vm-cri-import-image cri-resmgr-webhook "webhook/$(basename "$webhook_image_tar")"
    # Create a self-signed certificate with SANs
    vm-command "openssl req -x509 -newkey rsa:2048 -sha256 -days 365 -nodes -keyout webhook/server-key.pem -out webhook/server-crt.pem -subj '/CN=${service}.${namespace}.svc' -addext 'subjectAltName=DNS:${service},DNS:${service}.${namespace},DNS:${service}.${namespace}.svc'" ||
        command-error "creating self-signed certificate failed, requires openssl >= 1.1.1"
    # Allow webhook to run on node tainted by cmk=true
    sed -e "s|IMAGE_PLACEHOLDER|$webhook_image_repotag|" \
        -e 's|^\(\s*\)tolerations:$|\1tolerations:\n\1  - {"key": "cmk", "operator": "Equal", "value": "true", "effect": "NoSchedule"}|g' \
        -e 's/imagePullPolicy: Always/imagePullPolicy: Never/' \
        < "${HOST_PROJECT_DIR}/cmd/cri-resmgr-webhook/webhook-deployment.yaml" \
        | vm-pipe-to-file webhook/webhook-deployment.yaml
    # Create secret that contains svc.crt and svc.key for webhook deployment
    local server_crt_b64 server_key_b64
    server_crt_b64="$(vm-command-q "cat webhook/server-crt.pem" | base64 -w 0)"
    server_key_b64="$(vm-command-q "cat webhook/server-key.pem" | base64 -w 0)"
    cat <<EOF | vm-pipe-to-file --append webhook/webhook-deployment.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: cri-resmgr-webhook-secret
  namespace: cri-resmgr
data:
  svc.crt: ${server_crt_b64}
  svc.key: ${server_key_b64}
type: Opaque
EOF
    local cabundle_b64
    cabundle_b64="$server_crt_b64"
    sed -e "s/CA_BUNDLE_PLACEHOLDER/${cabundle_b64}/" \
        < "${HOST_PROJECT_DIR}/cmd/cri-resmgr-webhook/mutating-webhook-config.yaml" \
        | vm-pipe-to-file webhook/mutating-webhook-config.yaml
}

vm-pkg-type() {
    distro-pkg-type
}

vm-install-pkg() {
    distro-install-pkg "$@"
}

vm-setup-oneshot() {
    local util
    ( distro-refresh-pkg-db ) || true
    distro-setup-oneshot
    distro-install-utils
    # Verify that all required utilities exit on the VM.
    for util in pidof killall; do
        vm-command-q "command -v $util >/dev/null" || {
            error "required command '$util' missing on VM, fix/implement $distro-install-utils()"
        }
    done
}

vm-install-golang() {
    distro-install-golang
}

vm-install-runc() {
    local host_runc="$runc_src/runc"
    local vm_runc="/usr/sbin/runc"
    if [ -n "$runc_src" ]; then
        # Check if runc is already installed on VM.
        # If it is, replace existing binary with local build."
        vm-command 'command -v runc'
        if [ -n "$COMMAND_OUTPUT" ] && [ "x$COMMAND_STATUS" == "x0" ]; then
            vm_runc="$COMMAND_OUTPUT"
        fi
        vm-put-file "$host_runc" "$vm_runc"
    else
        distro-install-runc
    fi
}

vm-install-cri() {
    local vm_cri_dir="/usr/bin"
    distro-install-"$VM_CRI"
    distro-config-"$VM_CRI"
    if [ "$VM_CRI" == "containerd" ]; then
        if [ -n "$containerd_src" ]; then
            vm-command "systemctl stop containerd"
            vm-command 'command -v containerd'
            if [ -n "$COMMAND_OUTPUT" ] && [ "x$COMMAND_STATUS" == "x0" ]; then
                vm_cri_dir="${COMMAND_OUTPUT%/*}"
            fi
            for f in ctr containerd containerd-stress containerd-shim containerd-shim-runc-v1 containerd-shim-runc-v2; do
                vm-put-file "$containerd_src/bin/$f" "$vm_cri_dir/$f"
            done
            vm-command "systemctl enable --now containerd"
        fi
    elif [ "$VM_CRI" == "crio" ]; then
        if [ -n "$crio_src" ]; then
            vm-command "systemctl stop crio"
            vm-command 'command -v crio'
            if [ -n "$COMMAND_OUTPUT" ] && [ "x$COMMAND_STATUS" == "x0" ]; then
                vm_cri_dir="${COMMAND_OUTPUT%/*}"
            fi
            for f in crio crio-status pinns; do
                vm-put-file "$crio_src/bin/$f" "$vm_cri_dir/$f"
            done
            vm-command "systemctl enable --now crio"
        fi
    fi
}

vm-install-containernetworking() {
    vm-install-golang
    vm-command "GO111MODULE=off go get -d github.com/containernetworking/plugins"
    CNI_PLUGINS_SOURCE_DIR="$(awk '/package.*plugins/{print $NF}' <<< "$COMMAND_OUTPUT")"
    [ -n "$CNI_PLUGINS_SOURCE_DIR" ] || {
        command-error "downloading containernetworking plugins failed"
    }
    vm-command "pushd \"$CNI_PLUGINS_SOURCE_DIR\" && ./build_linux.sh && mkdir -p /opt/cni && cp -rv bin /opt/cni && popd" || {
        command-error "building and installing cri-tools failed"
    }
    vm-command "rm -rf /etc/cni/net.d && mkdir -p /etc/cni/net.d && cat > /etc/cni/net.d/10-bridge.conf <<EOF
{
  \"cniVersion\": \"0.4.0\",
  \"name\": \"mynet\",
  \"type\": \"bridge\",
  \"bridge\": \"cni0\",
  \"isGateway\": true,
  \"ipMasq\": true,
  \"ipam\": {
    \"type\": \"host-local\",
    \"subnet\": \"$CNI_SUBNET\",
    \"routes\": [
      { \"dst\": \"0.0.0.0/0\" }
    ]
  }
}
EOF"
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

vm-install-dlv() {
    vm-install-golang
    vm-install-pkg rsync
    vm-command "go install github.com/go-delve/delve/cmd/dlv@latest" || {
        command-error "installing delve failed"
    }
    echo '[ "`id -u`" -eq 0 ] && PATH=$PATH:/root/go/bin' | vm-pipe-to-file /etc/profile.d/root-path-go.sh
    vm-command "mkdir -p \"\$HOME/.config/dlv/config.yml.d\""
    vm-command "echo 'substitute-path:' > \"\$HOME/.config/dlv/config.yml.d/00-substitute-path\""
}

vm-install-glibc() { # script API
    # Usage: vm-install-glibc [VERSION]
    #
    # If glibc_src=/host/path/to/glibc is set, install a glibc that is
    # built and installed on host using configure --prefix $glibc_src.
    # If glibc_src is not set, download, build and install a glibc on vm.
    # In both cases glibc is installed to /opt/glibc/VERSION on vm.
    #
    # vm-set-glibc wraps selected binaries to use an installed glibc.
    #
    # Example: install a glibc from host and use it with two binaries.
    #   glibc_src=/host/glibc/install/prefix vm-install-glibc host-2.34
    #   vm-set-glibc host-2.34 /usr/bin/containerd /usr/local/bin/cri-resmgr
    #
    # Example: download, build and install glibc 2.32 on vm:
    #   vm-install-glibc 2.32
    #   vm-set-glibc 2.32 /usr/bin/containerd /usr/local/bin/cri-resmgr
    local glibc_ver="${1:-host}"
    local vm_glibc_dir="/opt/glibc/${glibc_ver}"
    if [ -n "$glibc_src" ] && [ -d "$glibc_src" ]; then
        vm-command "mkdir -p $vm_glibc_dir"
        ( cd "$glibc_src" && tar cz . ) | vm-pipe-to-file "$vm_glibc_dir/glibc-$glibc_ver.tar.gz" ||
            error "failed to package glibc from '$glibc_src'"
        vm-command "cd $vm_glibc_dir && tar xf glibc-$glibc_ver.tar.gz && rm -f glibc-$glibc_ver.tar.gz" ||
            command-error "failed to extract glibc-$glibc_ver.tar.gz"
        return 0
    fi
    if [[ "$glibc_ver" == "host"* ]]; then
        error "vm-install-glibc: invalid glibc_src='$glibc_src' when installing glibc from host"
    fi
    local vm_glibc_src="$vm_glibc_dir/src/glibc-${glibc_ver}"
    local vm_glibc_build="$vm_glibc_dir/src/build"
    local vm_glibc_install="$vm_glibc_dir"
    vm-install-pkg make bison flex gcc
    vm-command "mkdir -p $vm_glibc_src; cd $vm_glibc_src; curl -L --remote-name-all https://ftp.gnu.org/gnu/glibc/glibc-${glibc_ver}.tar.gz" ||
        command-error "failed to download glibc"
    vm-command "mkdir -p $vm_glibc_src; cd $vm_glibc_src/..; tar xzf $vm_glibc_src/glibc-${glibc_ver}.tar.gz" ||
        command-error "failed to extract glibc"
    vm-command "mkdir -p $vm_glibc_build; cd $vm_glibc_build && $vm_glibc_src/configure --prefix=$vm_glibc_install" ||
        command-error "failed to configure glibc"
    vm-command "cd $vm_glibc_build && make -j 4 >make.output.txt 2>&1 || ( tail make.output.txt; exit 1 )" ||
        command-error "failed to build glibc, see $vm_glibc_build/make.output.txt"
    vm-command "cd $vm_glibc_build && make install" ||
        command-error "failed to install glibc"
}

vm-set-glibc() { # script API
    # Usage: vm-set-glibc VERSION BIN [BIN...]
    #
    # Wrap binaries to use glibc VERSION.
    #
    # Note glibc VERSION must be installed first.
    # See vm-install-glibc.
    local glibc_ver="$1"
    local vm_glibc_dir="/opt/glibc/${glibc_ver}"
    local vm_glibc_install="$vm_glibc_dir"
    local vm_glibc_ld="$vm_glibc_install/lib/ld-linux-x86-64.so.2"
    shift
    if [ -z "$glibc_ver" ]; then
        error "vm-switch-glibc: missing glibc version to switch to"
    fi
    vm-command "[ -x $vm_glibc_ld ]" ||
        command-error "cannot find loader $vm_glibc_ld"
    local vm_bin
    for vm_bin in "$@"; do
        vm-command "[ -x $vm_bin ]" ||
            command-error "cannot find binary to be wrapped: $vm_bin"
        vm-command "( [ \"\$(dd bs=1 count=3 skip=1 if=$vm_bin)\" == \"ELF\" ] && mv $vm_bin ${vm_bin}.bin ) || [ -f $vm_bin.bin ]" ||
            command-error "failed to rename binary"
        vm-pipe-to-file "$vm_bin" <<EOF
#!/bin/bash
LD_LIBRARY_PATH=$vm_glibc_install/lib:\$LD_LIBRARY_PATH exec $vm_glibc_ld ${vm_bin}.bin "\$@"
EOF
        vm-command "chmod a+rx $vm_bin"
    done
}

vm-dlv-add-src() {
    local host_src_dir="$1"
    [ -d "$host_src_dir" ] || error "vm-dlv-add-src: invalid source directory \"$host_src_dir\", existing go project directory expected"
    vm-command "mkdir -p /home/$VM_SSH_USER/src; chmod a+rwX /home/$VM_SSH_USER/src; mkdir -p \$HOME/.config/dlv/config.yml.d"
    host-command "cd \"$host_src_dir/..\" && rsync -avz --include \"*/\" --include \"**/*.go\" --exclude \"*\" \"$(basename "$host_src_dir")\" $VM_SSH_USER@$VM_IP:src/"
    vm-command "echo ' - {from: \"$host_src_dir\", to: \"/home/$VM_SSH_USER/src/$(basename "$host_src_dir")\"}' > \"\$HOME/.config/dlv/config.yml.d/01-$(basename "$host_src_dir")\""
    vm-dlv-update-config
}

vm-dlv-update-config() {
    vm-command "( echo 'substitute-path:'; cat \$HOME/.config/dlv/config.yml.d/* ) > \$HOME/.config/dlv/config.yml"
}

vm-install-k8s() {
    distro-install-k8s
    distro-restart-$VM_CRI
}

vm-install-minikube() {
    vm-install-containernetworking
    distro-install-cri-dockerd
    distro-install-minikube
}

vm-create-minikube-cluster() {
    vm-command "sysctl fs.protected_regular=0; minikube start --driver=none --alsologtostderr=true"
}

vm-create-singlenode-cluster() {
    if ! [ "$(type -t vm-install-cni-$(distro-k8s-cni))" == "function" ]; then
        error "invalid CNI: $(distro-k8s-cni)"
    fi
    vm-create-cluster
    vm-command "kubectl taint nodes --all node-role.kubernetes.io/control-plane-"
    vm-command "kubectl taint nodes --all node-role.kubernetes.io/master-"
    vm-install-cni-"$(distro-k8s-cni)"
    if ! vm-command "kubectl wait --for=condition=Ready node/\$(hostname) --timeout=240s"; then
        command-error "kubectl waiting for node readiness timed out"
    fi
}

vm-create-cluster() {
    vm-command "kubeadm init --pod-network-cidr=$CNI_SUBNET --cri-socket ${k8scri_sock}"
    if ! grep -q "initialized successfully" <<< "$COMMAND_OUTPUT"; then
        command-error "kubeadm init failed"
    fi

    user="$(vm-ssh-user)"

    vm-command "mkdir -p ~$user/.kube"
    vm-command "cp /etc/kubernetes/admin.conf ~$user/.kube/config"
    vm-command "chown -R $user:$user ~$user/.kube"
    vm-command "mkdir -p ~root/.kube"
    vm-command "cp /etc/kubernetes/admin.conf ~root/.kube/config"
}

vm-destroy-cluster() {
    user="$(vm-ssh-user)"
    vm-command "yes | kubeadm reset; rm -f ~$user/.kube/config ~root/.kube/config /etc/kubernetes"
}

vm-install-cni-cilium() {
    if ! vm-command "curl -L --remote-name-all https://github.com/cilium/cilium-cli/releases/latest/download/cilium-linux-amd64.tar.gz && tar xzvfC cilium-linux-amd64.tar.gz /usr/local/bin && cilium install && rm -f cilium-linux-amd64.tar.gz"; then
        command-error "installing cilium CNI to Kubernetes failed"
    fi
}

vm-install-cni-weavenet() {
    vm-command "kubectl apply -f https://github.com/weaveworks/weave/releases/download/v2.8.1/weave-daemonset-k8s.yaml"
    if ! vm-command "kubectl rollout status --timeout=360s -n kube-system daemonsets/weave-net"; then
        command-error "installing weavenet CNI to Kubernetes failed/timed out"
    fi
}

vm-install-cni-flannel() {
    vm-command "kubectl apply -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml"
    if ! vm-command "kubectl rollout status --timeout=360s -n kube-system daemonsets/kube-flannel-ds"; then
        command-error "installing flannel CNI to Kubernetes failed/timed out"
    fi
}

vm-install-kernel-dev() { # script API
    # Usage: vm-install-kernel-dev
    #
    # Install dependencies and kernel sources ready for patching,
    # configuring and building packages.
    distro-install-kernel-dev
}

vm-print-usage() {
    echo "- Login VM:     ssh $VM_SSH_USER@$VM_IP"
    echo "- Stop VM:      govm stop $VM_NAME"
    echo "- Delete VM:    govm delete $VM_NAME"
}

vm-check-env || exit 1
