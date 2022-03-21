install-bfq() {
    # Install a kernel with BFQ I/O scheduler.
    if [[ "$distro" == ubuntu* ]]; then
        vm-command "uname -a"
        if ! grep -q lowlatency <<< "$COMMAND_OUTPUT"; then
            vm-command "apt install -y linux-image-lowlatency" ||
                command-error "failed to install lowlatency kernel for the BFQ I/O scheduler"

        fi
        vm-reboot
        vm-command-q "uname -a | grep lowlatency" || {
            error "failed to switch to lowlatency kernel"
        }
        vm-command "modprobe bfq"
    elif [[ "$distro" == debian* ]]; then
        vm-command "apt install -y linux-image-rt-amd64"
        vm-reboot
        vm-command "modprobe bfq"
    else
        error "not implemented: install kernel with BFQ I/O scheduler support to $distro"
    fi
}

# Make sure the BFQ scheduler is available in the system.
if ! vm-command-q "grep bfq /sys/block/vda/queue/scheduler"; then
    vm-command "modprobe bfq"
    vm-command-q "grep bfq /sys/block/vda/queue/scheduler" || {
        install-bfq
        vm-command-q "grep bfq /sys/block/vda/queue/scheduler" || {
            error "failed to make bfq I/O scheduler available in /sys/block/vda/queue/scheduler"
        }
    }
fi

# Switch to BFQ.
for blkdev in vda vdb; do
    if ! vm-command-q "grep '[[]bfq[]]' /sys/block/$blkdev/queue/scheduler"; then
        vm-command "echo bfq > /sys/block/$blkdev/queue/scheduler"
        vm-command-q "grep '[[]bfq[]]' /sys/block/$blkdev/queue/scheduler" || {
            error "failed to switch using bfq on /dev/$blkdev"
        }
    fi
done

if [[ "$k8scri" == *"containerd"* ]]; then
    # Start importing configurations from /etc/containerd/config.d/*.toml.
    vm-command-q "[ -f /etc/containerd/config.toml ] || echo "" > /etc/containerd/config.toml"
    vm-command-q "grep '^imports' /etc/containerd/config.toml || sed -i '1iimports = [\"/etc/containerd/config.d/*.toml\"]' /etc/containerd/config.toml"
    vm-command-q "grep -E '^imports.*/etc/containerd/config.d/' || sed -i 's:^\(imports.*\)\]:\1, \"/etc/containerd/config.d/*.toml\"\]:' /etc/containerd/config.toml"
    # e2e-specific config: tasks-service plugin loads blockio_config_file.
    vm-pipe-to-file /etc/containerd/config.d/e2e.toml <<EOF
[plugins."io.containerd.service.v1.tasks-service"]
blockio_config_file="/etc/containers/blockio.yaml"
EOF
    vm-command "systemctl restart containerd"
    sleep 5
else
    vm-command "systemctl restart crio" # make sure to apply latest --blockio-config-file
    sleep 5
fi

# Install a test script for viewing cgroups v1 and v2 values of k8s pods/containers.
vm-put-file "$HOST_PROJECT_DIR/scripts/testing/kube-cgroups" "/usr/local/bin/kube-cgroups"

# Create pod0, a single-container pod with a pod-level annotation.
ANN0='blockio.resources.beta.kubernetes.io/pod: "lowprio"' create besteffort

vm-command "kube-cgroups -f io\\. -p pod0"
# Check the blkio controller data when using cgroups v1.
if \
    grep blkio.throttle <<< "$COMMAND_OUTPUT" && (
        ( ! grep -A2 'blkio.throttle.read_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 1000000' ) || \
            ( ! grep -A2 'blkio.throttle.read_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 1000000' ) || \
            ( ! grep -A2 'blkio.throttle.read_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 2000' ) || \
            ( ! grep -A2 'blkio.throttle.read_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 2000' ) || \
            ( ! grep -A2 'blkio.throttle.write_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 512000' ) || \
            ( ! grep -A2 'blkio.throttle.write_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 512000' ) || \
            ( ! grep -A2 'blkio.throttle.write_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 300' ) || \
            ( ! grep -A2 'blkio.throttle.write_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 300' ) \
            )
then
    command-error "expected blkio.throttle.read_bps_device not found from cgroups v1"
fi

# Check the io controller data (successor of blkio) when using cgroups v2.
if \
    grep io.max <<< "$COMMAND_OUTPUT" && (
        ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 .*rbps=1000000' ) || \
            ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 .*rbps=1000000' ) || \
            ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 .*riops=2000' ) || \
            ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 .*riops=2000' ) || \
            ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 .*wbps=512000' ) || \
            ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 .*wbps=512000' ) || \
            ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 .*wiops=300' ) || \
            ( ! grep -A2 'io.max:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 .*wiops=300' ) \
            )
then
    command-error "expected io.max values not found from cgroups v2"
fi


# Create pod1, a triple-container pod with all containers in different blockio classes.
CONTCOUNT=3 CPU=1 MEM=64M \
         ANN0='blockio.resources.beta.kubernetes.io/pod: "lowprio"' \
         ANN1='blockio.resources.beta.kubernetes.io/container.pod1c1: "normal"' \
         ANN2='blockio.resources.beta.kubernetes.io/container.pod1c2: "highprio"' \
         create guaranteed

vm-command "kube-cgroups -f '.*io.bfq.weight.*' -c pod1c0"
grep -q 88 <<< "$COMMAND_OUTPUT" || error "expected blkio.bfq.weight: 88"

# In some setups besteffort slice has the default bfq weight 10, in some other 100. Accept both.
vm-command "kube-cgroups -f '.*io.bfq.weight.*' -c pod1c1"
grep -q -E 'default 10(0)?' <<< "$COMMAND_OUTPUT" || error "expected blkio.bfq.weight: default 10 or 100"

# Check device-specific bfq I/O scheduler weights.
vm-command "kube-cgroups -f '.*io.bfq.weight.*' -c pod1c2"
if grep -q blkio.bfq.weight_device <<< "$COMMAND_OUTPUT"; then
    grep -A3 blkio.bfq.weight_device <<< "$COMMAND_OUTPUT" | grep -q -E "25[0-5]:0 444" || error "expected 444"
    grep -A3 blkio.bfq.weight_device <<< "$COMMAND_OUTPUT" | grep -q -E "25[0-5]:16 555" || error "expected 555"
else
    grep -A3 io.bfq.weight <<< "$COMMAND_OUTPUT" | grep -q -E "25[0-5]:0 444" || error "expected 444"
    grep -A3 io.bfq.weight <<< "$COMMAND_OUTPUT" | grep -q -E "25[0-5]:16 555" || error "expected 555"
fi
