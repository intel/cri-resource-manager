#!/bin/bash

DEMO_TITLE="CRI Resource Manager: Block I/O Demo"

PV='pv -qL'

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
LIB_DIR=$SCRIPT_DIR/../lib
BIN_DIR=${bindir-$(realpath "$SCRIPT_DIR/../../bin")}
OUTPUT_DIR=${outdir-$SCRIPT_DIR/output}
COMMAND_OUTPUT_DIR=$OUTPUT_DIR/commands

source $LIB_DIR/command.bash
source $LIB_DIR/host.bash
source $LIB_DIR/vm.bash

usage() {
    echo "$DEMO_TITLE"
    echo "Usage: [VAR=VALUE] ./run.sh MODE"
    echo "  MODE:     \"play\" plays the demo."
    echo "            \"record\" plays and records the demo."
    echo "            \"test\" runs fast, reports pass or fail."
    echo "  VARs:"
    echo "    vm:      govm virtual machine name."
    echo "             The default is \"crirm-demo-blockio\"."
    echo "    speed:   Demo play speed."
    echo "             The default is 10 (keypresses per second)."
    echo "    cleanup: 0: leave VM running. (\"play\" mode default)"
    echo "             1: delete VM (\"test\" mode default)"
    echo "             2: stop VM, but do not delete it."
    echo "    outdir:  Save output under given directory."
    echo "             The default is \"${SCRIPT_DIR}/output\"."
    echo "    binsrc:  Where to get cri-resmgr to the VM."
    echo "             \"github\": go get and build in VM (\"play\" mode default)."
    echo "             \"local\": copy from source tree bin/ (\"test\" mode default)"
    echo "                      (set bindir=/path/to/cri-resmgr* to override bin/)"
}

error() {
    (echo ""; echo "error: $1" ) >&2
    exit 1
}

out() {
    if [ -n "$PV" ]; then
        speed=${speed-10}
        echo "$1" | $PV $speed
    else
        echo "$1"
    fi
    echo ""
}

record() {
    clear
    out "Recording this screencast..."
    host-command "asciinema rec -t \"$DEMO_TITLE\" crirm-demo-blockio.cast -c \"./run.sh play\""
}

screen-create-vm() {
    speed=60 out "### Running the demo in VM \"$vm\"."
    host-create-vm $vm
    vm-networking
    if [ -z "$VM_IP" ]; then
        error "creating VM failed"
    fi
}

screen-install-k8s() {
    speed=60 out "### Installing Kubernetes to the VM."
    vm-install-containerd
    vm-install-k8s
}

screen-install-cri-resmgr() {
    prefix=/usr/local
    speed=60 out "### Installing CRI Resource Manager to VM."
    if [ "$binsrc" == "github" ]; then
        vm-command "apt install -y golang make"
        vm-command "go get -d -v github.com/intel/cri-resource-manager"
        CRI_RESMGR_SOURCE_DIR=$(awk '/package.*cri-resource-manager/{print $NF}' <<< "$COMMAND_OUTPUT")
        vm-command "cd $CRI_RESMGR_SOURCE_DIR && make install && cd -"
    elif [ "$binsrc" == "local" ]; then
        host-command "scp \"$BIN_DIR/cri-resmgr\" \"$BIN_DIR/cri-resmgr-agent\" $VM_SSH_USER@$VM_IP:" || {
            command-error "copying local cri-resmgr to VM failed"
        }
        vm-command "mv cri-resmgr cri-resmgr-agent $prefix/bin" || {
            command-error "installing cri-resmgr to $prefix/bin failed"
        }
    fi
}

screen-launch-cri-resmgr() {
    policy=${policy-none}
    speed=60 out "### Launching cri-resmgr."
    vm-command "(echo \"policy:\"; echo \"  Active: $policy\") > cri-resmgr.fallback.cfg"
    vm-command "cri-resmgr -relay-socket /var/run/cri-resmgr/cri-resmgr.sock -runtime-socket /var/run/containerd/containerd.sock -fallback-config cri-resmgr.fallback.cfg >cri-resmgr.output.txt 2>&1 &"
}

screen-create-singlenode-cluster() {
    speed=60 out "### Setting up single-node Kubernetes cluster."
    speed=60 out "### CRI Resource Manager + containerd will act as the container runtime."
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

screen-launch-cri-resmgr-agent() {
    speed=60 out "### Launching cri-resmgr-agent."
    speed=60 out "### The agent will make cri-resmgr configurable with ConfigMaps."
    vm-command "NODE_NAME=\$(hostname) cri-resmgr-agent -kubeconfig \$HOME/.kube/config >cri-resmgr-agent.output.txt 2>&1 &"
}

screen-measure-io-speed() {
    process=$1
    measuretime=2
    out "### Measuring $process read speed -- twice."
    cmd="pid=\$(ps -A | awk \"/$process/{print \\\$1}\"); [ -n \"\$pid\" ] && { echo \$(grep read_bytes /proc/\$pid/io; sleep $measuretime; grep read_bytes /proc/\$pid/io) | awk \"{print \\\"$process read speed: \\\"(\\\$4-\\\$2)/$measuretime/1024\\\" kBps\\\"}\"; }"
    speed=360 outcolor=10 vm-command "$cmd"
    sleep 1
    speed=360 outcolor=10 vm-command "$cmd"
}

demo-blockio() {
    out "### Let the show begin!"
    out "### Configuring cri-resmgr: introduce a SlowReader block I/O class."
    host-command "scp cri-resmgr-config.default.yaml $VM_SSH_USER@$VM_IP:"
    vm-command "cat cri-resmgr-config.default.yaml"
    out "### Note: SlowReaders can read from each of the listed devices up to $(vm-command-q "awk '/ThrottleRead/{print \$2}' < cri-resmgr-config.default.yaml")Bps."
    vm-command "kubectl apply -f cri-resmgr-config.default.yaml"
    out "### Our test workload, bb-scanner, is annotated as a SlowReader."
    host-command "scp bb-scanner.yaml $VM_SSH_USER@$VM_IP:"
    vm-command "grep -A1 annotations: bb-scanner.yaml"
    out "### Flushing caches and deploying bb-scanner."
    vm-command "echo 3 > /proc/sys/vm/drop_caches"
    vm-command "kubectl create -f bb-scanner.yaml"

    out "### Now bb-scanner is running md5sum to all mounted directories, non-stop."
    vm-wait-process md5sum 60

    screen-measure-io-speed md5sum

    out "### Reconfiguring cri-resmgr: set SlowReader read speed to 2 MBps."
    out "### This applies to all pods and containers in this block I/O class,"
    out "### both new and already running, like our bb-scanner."
    vm-command "sed -i 's/ThrottleReadBps:.*/ThrottleReadBps: 2Mi/' cri-resmgr-config.default.yaml"
    vm-command "cat cri-resmgr-config.default.yaml"
    vm-command "kubectl apply -f cri-resmgr-config.default.yaml"

    # Give some time for new config to become effective and process
    # I/O to accelerate.
    sleep 2;

    screen-measure-io-speed md5sum

    out "### Thanks for watching!"
    out "### Cleaning up: deleting bb-scanner."
    vm-command "kubectl delete daemonset bb-scanner"
}

# Validate parameters
mode=$1
vm=${vm-"crirm-demo-blockio"}

if [ "$mode" == "play" ]; then
    speed=${speed-10}
    cleanup=${cleanup-0}
    binsrc=${binsrc-github}
elif [ "$mode" == "test" ]; then
    PV=
    cleanup=${cleanup-1}
    binsrc=${binsrc-local}
elif [ "$mode" == "record" ]; then
    record
else
    usage
    error "missing valid MODE"
    exit 1
fi

# Prepare for test/demo
mkdir -p $OUTPUT_DIR
mkdir -p $COMMAND_OUTPUT_DIR
rm -f $COMMAND_OUTPUT_DIR/0*
( echo x > $OUTPUT_DIR/x && rm -f $OUTPUT_DIR/x ) || {
    error "output directory outdir=$OUTPUT_DIR is not writable"
}

if [ "$binsrc" == "local" ]; then
    [ -f "${BIN_DIR}/cri-resmgr" ] || error "missing \"${BIN_DIR}/cri-resmgr\""
    [ -f "${BIN_DIR}/cri-resmgr-agent" ] || error "missing \"${BIN_DIR}/cri-resmgr-agent\""
fi

if [ -z "$VM_IP" ] || [ -z "$VM_SSH_USER" ] || [ -z "$VM_NAME" ]; then
    screen-create-vm
fi

if ! vm-command-q "dpkg -l | grep -q kubelet"; then
    screen-install-k8s
fi

if ! vm-command-q "[ -f /usr/bin/cri-resmgr ]"; then
    screen-install-cri-resmgr
fi

# start cri-resmgr if not already running
if ! vm-command-q "pidof cri-resmgr" >/dev/null; then
    screen-launch-cri-resmgr
fi

# create kubernetes cluster or wait that it is online
if vm-command-q "[ ! -f /var/lib/kubelet/config.yaml ]"; then
    screen-create-singlenode-cluster
else
    # wait for kube-apiserver to launch (may be down if the VM was just booted)
    vm-wait-process kube-apiserver
fi

# start cri-resmgr-agent if not already running
if ! vm-command-q "pidof cri-resmgr-agent >/dev/null"; then
    screen-launch-cri-resmgr-agent
fi

# Run test/demo
demo-blockio

# Cleanup
if [ "$cleanup" == "0" ]; then
    echo "The VM, Kubernetes and cri-resmgr are left running. Next steps:"
    vm-print-usage
elif [ "$cleanup" == "1" ]; then
    host-stop-vm $vm
    host-delete-vm $vm
elif [ "$cleanup" == "2" ]; then
    host-stop-vm $vm
fi

# Summarize results
SUMMARY_FILE="$OUTPUT_DIR/summary.txt"
echo -n "" > "$SUMMARY_FILE" || error "cannot write summary to \"$SUMMARY_FILE\""
first_speed=$(grep "^md5sum read speed:" $COMMAND_OUTPUT_DIR/0* | head -n 1 | awk '{print $4}')
last_speed=$(grep "^md5sum read speed:" $COMMAND_OUTPUT_DIR/0* | tail -n 1 | awk '{print $4}')
echo "First md5sum read speed (512 kBps throttling): $first_speed kBps" >> "$SUMMARY_FILE"
echo "Last  md5sum read speed (2 MBps throttling): $last_speed kBps" >> "$SUMMARY_FILE"
# Declare verdict in test mode
exit_status=0
if [ "$mode" == "test" ]; then
    min_first=100 max_first=600 min_last=1500 max_last=2500
    [[ "$first_speed" -gt "$min_first" ]] || exit_status=1
    [[ "$first_speed" -lt "$max_first" ]] || exit_status=1
    [[ "$last_speed" -gt "$min_last" ]] || exit_status=1
    [[ "$last_speed" -lt "$max_last" ]] || exit_status=1
    if [ "$exit_status" == "1" ]; then
        echo "Error: speeds outside acceptable ranges ($min_first..$max_first kBps and $min_last..$max_last kBps)." >> "$SUMMARY_FILE"
        echo "Test verdict: FAIL" >> "$SUMMARY_FILE"
    else
        echo "Speeds within acceptable ranges ($min_first..$max_first kBps and $min_last..$max_last kBps)." >> "$SUMMARY_FILE"
        echo "Test verdict: PASS" >> "$SUMMARY_FILE"
    fi
    echo ""
    cat "$SUMMARY_FILE"
fi
exit $exit_status
