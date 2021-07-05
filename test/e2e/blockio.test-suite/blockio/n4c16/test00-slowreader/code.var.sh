install-bfq() {
    # Install a kernel with BFQ I/O scheduler
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

# Make sure the BFQ scheduler is available in the system
if ! vm-command-q "grep bfq /sys/block/vda/queue/scheduler"; then
    vm-command "modprobe bfq"
    vm-command-q "grep bfq /sys/block/vda/queue/scheduler" || {
        install-bfq
        vm-command-q "grep bfq /sys/block/vda/queue/scheduler" || {
            error "failed to make bfq I/O scheduler available in /sys/block/vda/queue/scheduler"
        }
    }
fi

# Switch to BFQ
for blkdev in vda vdb; do
    if ! vm-command-q "grep '[[]bfq[]]' /sys/block/$blkdev/queue/scheduler"; then
        vm-command "echo bfq > /sys/block/$blkdev/queue/scheduler"
        vm-command-q "grep '[[]bfq[]]' /sys/block/$blkdev/queue/scheduler" || {
            error "failed to switch using bfq on /dev/$blkdev"
        }
    fi
done

vm-command "systemctl restart crio" # make sure to apply latest --blockio-config-file

vm-put-file "$HOST_PROJECT_DIR/scripts/testing/kube-cgroups" "/usr/local/bin/kube-cgroups"

# pod0: single-container pod with a pod-level annotation
ANN0='blockio.resources.beta.kubernetes.io/pod: "lowprio"' create besteffort
vm-command "kube-cgroups -p pod0"
if \
    ( ! grep -A2 'blkio.throttle.read_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 1000000' ) || \
        ( ! grep -A2 'blkio.throttle.read_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 1000000' ) || \
        ( ! grep -A2 'blkio.throttle.read_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 2000' ) || \
        ( ! grep -A2 'blkio.throttle.read_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 2000' ) || \
        ( ! grep -A2 'blkio.throttle.write_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 512000' ) || \
        ( ! grep -A2 'blkio.throttle.write_bps_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 512000' ) || \
        ( ! grep -A2 'blkio.throttle.write_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:16 300' ) || \
        ( ! grep -A2 'blkio.throttle.write_iops_device:' <<< "$COMMAND_OUTPUT" | grep -q -E '25[0-5]:0 300' ) \
   ; then
    command-error "expected blkio.throttle.read_bps_device not found from cgroups"
fi

# pod1: triple-container pod with all containers in different blockio classes
CONTCOUNT=3 CPU=1 MEM=64M \
         ANN0='blockio.resources.beta.kubernetes.io/pod: "lowprio"' \
         ANN1='blockio.resources.beta.kubernetes.io/container.pod1c1: "normal"' \
         ANN2='blockio.resources.beta.kubernetes.io/container.pod1c2: "highprio"' \
         create guaranteed

vm-command "kube-cgroups -f blkio.bfq.weight.* -c pod1c0"
grep -q 88 <<< "$COMMAND_OUTPUT" || error "expected blkio.bfq.weight: 88"

vm-command "kube-cgroups -f blkio.bfq.weight.* -c pod1c1"
grep -q 100 <<< "$COMMAND_OUTPUT" || error "expected blkio.bfq.weight: 100"

vm-command "kube-cgroups -f blkio.bfq.weight.* -c pod1c2"
grep -A3 blkio.bfq.weight_device <<< "$COMMAND_OUTPUT" | grep -q -E "25[0-5]:0 444" || error "expected 444"
grep -A3 blkio.bfq.weight_device <<< "$COMMAND_OUTPUT" | grep -q -E "25[0-5]:16 555" || error "expected 555"
