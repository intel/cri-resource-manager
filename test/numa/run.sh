#!/bin/bash

DEMO_TITLE="CRI Resource Manager: Numa test"

PV='pv -qL'

binsrc=${binsrc-local}

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
DEMO_LIB_DIR=$(realpath "$SCRIPT_DIR/../../demo/lib")
BIN_DIR=${bindir-$(realpath "$SCRIPT_DIR/../../bin")}
OUTPUT_DIR=${outdir-$SCRIPT_DIR/output}
COMMAND_OUTPUT_DIR=$OUTPUT_DIR/commands

source $DEMO_LIB_DIR/command.bash
source $DEMO_LIB_DIR/host.bash
source $DEMO_LIB_DIR/vm.bash

usage() {
    echo "$DEMO_TITLE"
    echo "Usage: [VAR=VALUE] ./run.sh MODE"
    echo "  MODE:     \"play\" plays the test as a demo."
    echo "            \"record\" plays and records the demo."
    echo "            \"test\" runs fast, reports pass or fail."
    echo "  VARs:"
    echo "    vm:      govm virtual machine name."
    echo "             The default is \"crirm-test-numa\"."
    echo "    speed:   Demo play speed."
    echo "             The default is 10 (keypresses per second)."
    echo "    cleanup: Level of cleanup after test run:"
    echo "             0: leave VM running. (\"play\" mode default)"
    echo "             1: delete VM (\"test\" mode default)"
    echo "             2: stop VM, but do not delete it."
    echo "    outdir:  Save output under given directory."
    echo "             The default is \"${SCRIPT_DIR}/output\"."
    echo "    binsrc:  Where to get cri-resmgr to the VM."
    echo "             \"github\": go get from master and build inside VM."
    echo "             \"local\": copy from source tree bin/ (the default)."
    echo "                      (set bindir=/path/to/cri-resmgr* to override bin/)"
    echo "    reinstall_cri_resmgr: If 1, stop running cri-resmgr, reinstall,"
    echo "             and restart it on the VM before starting test run."
    echo ""
    echo "  Test input VARs:"
    echo "    numanodes: JSON to override NUMA node list used in tests."
    echo "             Effective only if \"vm\" does not exist."
    echo "    cri_resmgr_cfg: configuration file forced to cri-resmgr."
    echo "    pods:    JSON to override the default Pods to be created."
    echo "             The array specifies the order, and objects the"
    echo "             QoS class (guaranteed, besteffort or burstable)"
    echo "             and non-default values for variables in"
    echo "             QOSCLASS.yaml.in templates."
    echo ""
    echo "Default test input VARs: ./run.sh help defaults"
    echo ""
    echo "Development cycle example:"
    echo "pushd ../..; make; popd; reinstall_cri_resmgr=1 speed=120 ./run.sh play"
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
    speed=60 out "### Running the test in VM \"$vm\"."
    # Create a machine with 5 NUMA nodes.
    # Qemu default NUMA node self-distance is 10.
    # Define distance 22 between all 4 nodes with CPU(s).
    # The distance from nodes with CPU(s) and the node with NVRAM is 88.
    host-create-vm $vm "$numanodes"
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
    speed=60 out "### Installing CRI Resource Manager to VM."
    vm-install-cri-resmgr
}

screen-launch-cri-resmgr() {
    speed=120 out "### Launching cri-resmgr with config $cri_resmgr_cfg."
    host-command "scp $cri_resmgr_cfg $VM_SSH_USER@$VM_IP:" || {
        command-error "copying $cri_resmgr_cfg to VM failed"
    }
    vm-command "cat $(basename $cri_resmgr_cfg)"
    vm-command "cri-resmgr -relay-socket /var/run/cri-resmgr/cri-resmgr.sock -runtime-socket /var/run/containerd/containerd.sock -force-config $(basename $cri_resmgr_cfg) >cri-resmgr.output.txt 2>&1 &"
}

screen-create-singlenode-cluster() {
    speed=60 out "### Setting up single-node Kubernetes cluster."
    speed=60 out "### CRI Resource Manager + containerd will act as the container runtime."
    vm-create-singlenode-cluster-cilium
}

screen-launch-cri-resmgr-agent() {
    speed=60 out "### Launching cri-resmgr-agent."
    speed=60 out "### The agent will make cri-resmgr configurable with ConfigMaps."
    vm-command "NODE_NAME=\$(hostname) cri-resmgr-agent -kubeconfig \$HOME/.kube/config >cri-resmgr-agent.output.txt 2>&1 &"
}

create-wait() {
    # Copy NAME.yaml from host to vm, kubectl create -f it,
    # and wait for it becoming Ready.
    local NAME=$1
    local YAML_KIND=$(awk '/kind/{print $2}' < $NAME.yaml)
    host-command "scp $NAME.yaml $VM_SSH_USER@$VM_IP:" || {
        command-error "copying $NAME.yaml to VM failed"
    }
    vm-command "cat $NAME.yaml"
    vm-command "kubectl create -f $NAME.yaml"
    vm-command-q "kubectl wait --timeout=60s --for=condition=Ready ${YAML_KIND}/$NAME" >/dev/null 2>&1 || {
        command-error "waiting for ${YAML_KIND} \"$NAME\" to become ready timed out"
    }
}

get-cpus-allowed-mask() {
    local PROCESS_CMD_CONTAINS=$1
    vm-command "grep Cpus_allowed: /proc/\$(pgrep -f ${PROCESS_CMD_CONTAINS})/status | awk '{print \$2}'"
    local mask=$COMMAND_OUTPUT
    if [ "$(echo $mask | wc -c)" -gt "3" ] || [ "$(echo $mask | wc -c)" -lt "2" ]; then
        command-error "expected Cpus_allowed mask value, got \"$mask\""
    fi
    CPUS_ALLOWED_MASK=$(python3 -c "print(bin(0x$mask))")
}

test-exclusive-cpus() {
    vm-command-q "kubectl get pods 2>&1 | grep -q NAME" && vm-command "kubectl delete pods --all --now"

    # Instantiate QOSCLASS.yaml.in with parameters in pods JSON
    local last_pod_index=-1
    local pod_descr=()
    local pod_names=()
    local cpu_masks=()
    out "### Creating pods..."
    for pod_group_index in $(seq 0 $(jq length-1 <<<$pods)); do
        local pod_qos_class=$(jq -r ".[$pod_group_index].q" <<<$pods)
        local pod_count=$(jq -r ".[$pod_group_index].pods" <<<$pods)
        if [ "$pod_count" == "null" ]; then
            pod_count=1
        fi
        local pod_yaml_in=${pod_qos_class}.yaml.in
        if ! [ -f "$pod_yaml_in" ]; then
            error "cannot find pod yaml template $pod_yaml_in"
        fi
        local pod_env=$(jq -r ".[$pod_group_index]|to_entries|.[]|select(.key != \"q\" and .key != \"pods\")|\"\(.key)=\(.value)\"" <<<$pods)
        for pod_in_group in $(seq 1 $pod_count); do
            pod_index=$(( last_pod_index + 1 ))
            pod_descr[$pod_index]="$pod_qos_class $(echo $pod_env)"
            pod_env_defaults="NAME=pod${pod_index} ${yaml_in_defaults}"
            eval $pod_env_defaults
            eval $pod_env
            echo "### Creating Pod $NAME: ${pod_descr[$pod_index]}"
            eval "echo -e \"$(<${pod_yaml_in})\"" > $NAME.yaml
            create-wait $NAME || {
                error "failed to create pod $NAME from YAML: $(<$NAME.yaml)"
            }
            last_pod_index=$pod_index
            pod_names[$pod_index]=$NAME
        done
    done
    out "### All pods are now running."

    out "### Reading which CPUs each pod is allowed to run on."
    for pod_index in $(seq 0 $last_pod_index); do
        get-cpus-allowed-mask ${pod_names[$pod_index]}; cpu_masks[$pod_index]=$CPUS_ALLOWED_MASK
    done

    out "### Cpus_allowed masks:"
    out "$(printf "%-8s %15s  %s" "Pod" "Cpus_allowed" "Description")"
    for pod_index in $(seq 0 $last_pod_index); do
        out "$(printf "%-8s %15s  %s" ${pod_names[$pod_index]} ${cpu_masks[$pod_index]} "${pod_descr[$pod_index]}")"
    done
}

# Validate parameters
mode=$1
vm=${vm-"crirm-test-numa"}
cri_resmgr_cfg=${cri_resmgr_cfg-"cri-resmgr-memtier.cfg"}
numanodes=${numanodes-'[
    {"cpu": 1, "mem": "1G", "nodes": 2},
    {"cpu": 2, "mem": "2G"},
    {"cpu": 4, "mem": "4G"},
    {"nvmem": "8G",
     "dist": 22,
     "dist-group-0": 88,
     "dist-group-1": 88,
     "dist-group-2": 88
    }]'}
pods=${pods-'[
    {"q": "guaranteed", "CPU": 1},
    {"q": "besteffort", "pods": 2},
    {"q": "burstable", "CPULIM": 4, "CPUREQ": 2},
    {"q": "guaranteed", "CPU": 3}
    ]'}
yaml_in_defaults="CPU=1 MEM=100M ISO=true CPUREQ=1 CPULIM=2 MEMREQ=100M MEMLIM=200M"

if [ "$mode" == "help" ]; then
    if [ "$2" != "defaults" ]; then
        usage
    else
        echo "Test input defaults:"
        echo ""
        echo "numanodes=${numanodes}"
        echo ""
        echo "cri_resmgr_cfg=${cri_resmgr_cfg}"
        echo ""
        echo "pods=${pods}"
        echo ""
        echo "The defaults to QOSCLASS.yaml.in variables:"
        echo "    ${yaml_in_defaults}"
    fi
    exit 0
elif [ "$mode" == "play" ]; then
    speed=${speed-10}
    cleanup=${cleanup-0}
elif [ "$mode" == "test" ]; then
    PV=
    cleanup=${cleanup-1}
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

if [ "$reinstall_cri_resmgr" == "1" ]; then
    vm-command "kill -9 \$(pgrep cri-resmgr); rm -rf /usr/local/bin/cri-resmgr /usr/bin/cri-resmgr /usr/local/bin/cri-resmgr-agent /usr/bin/cri-resmgr-agent /var/lib/resmgr"
fi

if ! vm-command-q "[ -f /usr/local/bin/cri-resmgr ]"; then
    screen-install-cri-resmgr
fi

# Start cri-resmgr if not already running
if ! vm-command-q "pidof cri-resmgr" >/dev/null; then
    screen-launch-cri-resmgr
fi

# Create kubernetes cluster or wait that it is online
if vm-command-q "[ ! -f /var/lib/kubelet/config.yaml ]"; then
    screen-create-singlenode-cluster
else
    # Wait for kube-apiserver to launch (may be down if the VM was just booted)
    vm-wait-process kube-apiserver
fi

# Run test/demo
TEST_FAILURES=""
test-exclusive-cpus

# Save logs
host-command "scp $VM_SSH_USER@$VM_IP:cri-resmgr.output.txt \"$OUTPUT_DIR/\""

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
exit_status=0
if [ -n "$TEST_FAILURES" ]; then
    echo "FAIL:$TEST_FAILURES"
    exit_status=1
    exit $exit_status
fi
