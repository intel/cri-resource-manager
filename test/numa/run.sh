#!/bin/bash

DEMO_TITLE="CRI Resource Manager: Numa test"

PV='pv -qL'

binsrc=${binsrc-local}

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
DEMO_LIB_DIR=$(realpath "$SCRIPT_DIR/../../demo/lib")
BIN_DIR=${bindir-$(realpath "$SCRIPT_DIR/../../bin")}
OUTPUT_DIR=${outdir-"$SCRIPT_DIR"/output}
COMMAND_OUTPUT_DIR="$OUTPUT_DIR"/commands

# shellcheck disable=SC1091
# shellcheck source=../../demo/lib/command.bash
source "$DEMO_LIB_DIR"/command.bash
# shellcheck disable=SC1091
# shellcheck source=../../demo/lib/host.bash
source "$DEMO_LIB_DIR"/host.bash
# shellcheck disable=SC1091
# shellcheck source=../../demo/lib/vm.bash
source "$DEMO_LIB_DIR"/vm.bash

script_source="$(< "$0") $(< "$DEMO_LIB_DIR/host.bash") $(< "$DEMO_LIB_DIR/command.bash") $(< "$DEMO_LIB_DIR/vm.bash")"

usage() {
    echo "$DEMO_TITLE"
    echo "Usage: [VAR=VALUE] ./run.sh MODE [SCRIPT]"
    echo "  MODE:     \"play\" plays the test as a demo."
    echo "            \"record\" plays and records the demo."
    echo "            \"test\" runs fast, reports pass or fail."
    echo "            \"interactive\" launches interactive shell"
    echo "            for running test script commands"
    echo "            (see ./run.sh help script [FUNCTION])."
    echo "  SCRIPT:   test script file to run instead of the default test."
    echo ""
    echo "  VARs:"
    echo "    vm:      govm virtual machine name."
    echo "             The default is \"crirm-test-numa\"."
    echo "    speed:   Demo play speed."
    echo "             The default is 10 (keypresses per second)."
    echo "    binsrc:  Where to get cri-resmgr to the VM."
    echo "             \"github\": go get from master and build inside VM."
    echo "             \"local\": copy from source tree bin/ (the default)."
    echo "                      (set bindir=/path/to/cri-resmgr* to override bin/)"
    echo "    reinstall_cri_resmgr: If 1, stop running cri-resmgr, reinstall,"
    echo "             and restart it on the VM before starting test run."
    echo "             The default is 0."
    echo "    outdir:  Save output under given directory."
    echo "             The default is \"${SCRIPT_DIR}/output\"."
    echo "    cleanup: Level of cleanup after a test run:"
    echo "             0: leave VM running (the default)"
    echo "             1: delete VM"
    echo "             2: stop VM, but do not delete it."
    echo ""
    echo "  Test input VARs:"
    echo "    numanodes: JSON to override NUMA node list used in tests."
    echo "             Effective only if \"vm\" does not exist."
    echo "    cri_resmgr_cfg: configuration file forced to cri-resmgr."
    echo "    code:    Variable that contains test script code to be run"
    echo "             if SCRIPT is not given."
    echo ""
    echo "Default test input VARs: ./run.sh help defaults"
    echo ""
    echo "Development cycle example:"
    echo "pushd ../..; make; popd; reinstall_cri_resmgr=1 speed=120 ./run.sh play"
}

error() {
    (echo ""; echo "error: $1" ) >&2
    command-exit-if-not-interactive
}

out() {
    if [ -n "$PV" ]; then
        speed=${speed-10}
        echo "$1" | $PV "$speed"
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
    host-create-vm "$vm" "$numanodes"
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
    speed=60 out "### Launching cri-resmgr with config $cri_resmgr_cfg."
    host-command "scp \"$cri_resmgr_cfg\" $VM_SSH_USER@$VM_IP:" || {
        command-error "copying \"$cri_resmgr_cfg\" to VM failed"
    }
    vm-command "cat $(basename "$cri_resmgr_cfg")"
    vm-command "cri-resmgr -relay-socket /var/run/cri-resmgr/cri-resmgr.sock -runtime-socket /var/run/containerd/containerd.sock -force-config $(basename "$cri_resmgr_cfg") >cri-resmgr.output.txt 2>&1 &"
    sleep 2 >/dev/null 2>&1
    vm-command "grep 'FATAL ERROR' cri-resmgr.output.txt" >/dev/null 2>&1 && {
        command-error "launching cri-resmgr failed with FATAL ERROR"
    }
    vm-command "pidof cri-resmgr" >/dev/null 2>&1 || {
        command-error "launching cri-resmgr failed, cannot find cri-resmgr PID"
    }
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

get-py-cpus() {
    # Fetch Cpus_allowed masks of running pods from the virtual machine
    # and update them in the "cpus" dictionary in Python code.
    speed=1000 vm-command "echo cpus={}; for pod_name in \$(kubectl get pods | awk '/pod/{print \$1}'); do pod_index=\${pod_name/pod/}; mask=\$(grep Cpus_allowed: /proc/\$(pgrep -f \${pod_name})/status | awk '{print \$2}'); echo cpus[\$pod_index]=0x\$mask; done" >/dev/null 2>&1 || {
        command-error "error in reading Cpus_allowed masks from running pods"
    }
    if grep -q '=0' <<<"$COMMAND_OUTPUT"; then
        py_cpus="$COMMAND_OUTPUT"
    else
        py_cpus="cpus={}"
    fi
}

get-py-cache() {
    # Fetch current cri-resmgr cache from a virtual machine.
    speed=1000 vm-command "cat \"/var/lib/cri-resmgr/cache\"" >/dev/null 2>&1 || {
        command-error "fetching cache file failed"
    }
    cat > "${OUTPUT_DIR}/cache" <<<"$COMMAND_OUTPUT"
    py_cache="import json;cache=json.load(open(\"${OUTPUT_DIR}/cache\"));allocations=json.loads(cache['PolicyJSON']['allocations'])"
}

### Test script helpers

sleep() { # script API
    # Usage: sleep PARAMETERS
    #
    # Run sleep PARAMETERS on host.
    host-command "sleep $*"
}

pyexec() { # script API
    # Usage: pyexec [PYTHONCODE...]
    #
    # Run python3 -c PYTHONCODEs on host. Stops if execution fails.
    #
    # Variables available in PYTHONCODE:
    #   cpus:        dictionary: {pod_number: cpu_mask}
    #   cache:       dictionary, cri-resmgr cache
    #   allocations: dictionary: shorthand to cri-resmgr policy allocations
    #                (unmarshaled cache['PolicyJSON']['allocations'])
    #
    # Note that variables are *not* updated when pyexec is called.
    # You can update the variables by running "verify" without expressions.
    #
    # Example:
    #   verify ; pyexec 'import pprint; pprint.pprint(cpus)'
    PYEXEC_PY="$OUTPUT_DIR/pyexec.py"
    PYEXEC_LOG="$OUTPUT_DIR/pyexec.output.txt"
    local last_exit_status=0
    for PYTHONCODE in "$@"; do
        cat > "$PYEXEC_PY" <<<"$py_cpus"
        cat >> "$PYEXEC_PY" <<<"$py_cache"
        cat >> "$PYEXEC_PY" <<<"$PYTHONCODE"
        python3 "$PYEXEC_PY" 2>&1 | tee "$PYEXEC_LOG"
        last_exit_status=${PIPESTATUS[0]}
        if [ "$last_exit_status" != "0" ]; then
            error "pyexec: non-zero exit status \"$last_exit_status\", see \"$PYEXEC_PY\" and \"$PYEXEC_LOG\""
        fi
    done
    return "$last_exit_status"
}

report() { # script API
    # Usage: report [VARIABLE...]
    #
    # Updates and reports current value of VARIABLE.
    #
    # Example: print CPU masks of containers in pod0, pod1, ...
    #   report cpus
    #
    # Example: print cri-resmgr policy allocations. In interactive mode
    #          you may use a pager like less.
    #   report allocations | less
    local varname
    for varname in "$@"; do
        if [ "$varname" == "cpus" ]; then
            get-py-cpus
            pyexec "
import math
print('Pod   Cpus_allowed_mask')
if cpus:
    bits_needed=int(math.log2(max(cpus.values())))+1
    for podnum in sorted(cpus.keys()):
        print(('pod%d  %s') % (podnum, bin(cpus[podnum])[2:].zfill(bits_needed)))
"
        elif [ "$varname" == "allocations" ]; then
            get-py-cache
            pyexec "
import pprint
pprint.pprint(allocations)
"
        elif [ "$varname" == "cache" ]; then
            get-py-cache
            pyexec "
import pprint
pprint.pprint(cache)
"
        else
            error "report: unknown variable \"$varname\""
        fi
    done
}

verify() { # script API
    # Usage: verify [EXPR...]
    #
    # Run python3 -c "assert(EXPR)" to test that every EXPR is True.
    # Stop evaluation on the first EXPR not True and stops script.
    # You can allow script execution to continue after failed verification
    # by running verify in a subshell (in parenthesis):
    #   (verify 'False') || echo '...but was expected to fail.'
    #
    # Variables available in expressions:
    #   cpus: dictionary {pod_number: cpu_mask}
    #   cache:       dictionary, cri-resmgr cache
    #   allocations: dictionary: shorthand to cri-resmgr policy allocations
    #                (unmarshaled cache['PolicyJSON']['allocations'])
    #
    # Note that variables are updated every time verify is called
    # before evaluating (asserting) expressions.
    #
    # Example:
    #   require that pod0 and pod1 cpu masks are disjoint and that
    #   pod0 cpu mask has four 1's in it:
    #     verify 'cpus[0] & cpus[1] == 0' 'bin(cpus[0]).count("1") == 4'
    get-py-cpus
    get-py-cache
    for py_assertion in "$@"; do
        speed=1000 out "### Verifying assertion '$py_assertion'"
        ( speed=1000 pyexec "assert(${py_assertion})" ) || {
                out "### The assertion FAILED"
                echo "verify: assertion '$py_assertion' failed." >> "$SUMMARY_FILE"
                command-exit-if-not-interactive
        }
        speed=1000 out "### The assertion holds."
    done
}

delete() { # script API
    # Usage: delete PARAMETERS
    #
    # Run "kubectl delete PARAMETERS".
    vm-command "kubectl delete $*" || {
        command-error "kubectl delete failed"
    }
}

create() { # script API
    # Usage: [VAR=VALUE] create TEMPLATE_NAME
    #
    # Create n instances from TEMPLATE_NAME.yaml.in, copy each of them
    # from host to vm, kubectl create -f them, and wait for them
    # becoming Ready.
    #
    # Parameters:
    #   TEMPLATE_NAME: the name of the template without extension (.yaml.in)
    #
    # Optional parameters (VAR=VALUE):
    #   wait: condition to be waited for (see kubectl wait --for=condition=).
    #         If empty (""), skip waiting. The default is wait="Ready".
    #   wait_t: wait timeout. The default is wait_t=60s.
    template_name=$1.yaml.in
    local template_kind
    template_kind=$(awk '/kind/{print tolower($2)}' < "$template_name")
    local wait=${wait-Ready}
    local wait_t=${wait_t-60s}
    if [ -z "$n" ]; then
        local n=1
    fi
    if [ ! -f "$template_name" ]; then
        error "error creating \"$1\": missing template ${template_name}"
    fi
    for _ in $(seq 1 $n); do
        kind_count[$template_kind]=$(( ${kind_count[$template_kind]} + 1 ))
        local NAME="${template_kind}$(( ${kind_count[$template_kind]} - 1 ))" # the first pod is pod0
        eval "echo -e \"$(<"${template_name}")\"" > "$NAME.yaml"
        host-command "scp $NAME.yaml $VM_SSH_USER@$VM_IP:" || {
            command-error "copying $NAME.yaml to VM failed"
        }
        vm-command "cat $NAME.yaml"
        vm-command "kubectl create -f $NAME.yaml" || {
            command-error "kubectl create error"
        }
        if [ "x$wait" != "x" ]; then
            speed=1000 vm-command "kubectl wait --timeout=${wait_t} --for=condition=${wait} ${template_kind}/$NAME" >/dev/null 2>&1 || {
                command-error "waiting for ${template_kind} \"$NAME\" to become ready timed out"
            }
        fi
    done
}

interactive() { # script API
    # Usage: interactive
    #
    # Enter the interactive mode: read next script commands from
    # the standard input until "exit".
    echo "Entering the interactive mode until \"exit\"."
    INTERACTIVE_MODE=$(( INTERACTIVE_MODE + 1 ))
    # shellcheck disable=SC2162
    while read -e -p "run.sh> " -a commands; do
        if [ "${commands[0]}" == "exit" ]; then
            break
        fi
        eval "${commands[@]}"
    done
    INTERACTIVE_MODE=$(( INTERACTIVE_MODE - 1 ))
}

help() { # script API
    # Usage: help [FUNCTION|all]
    #
    # Print help on all functions or on the FUNCTION available in script.
    awk -v f="$1" \
        '/^[a-z].*script API/{split($1,a,"(");if(f==""||f==a[1]||f=="all"){print "";print a[1]":";l=2}}
         !/^    #/{l=l-1}
         /^    #/{if(l>=1){split($0,a,"#"); print "   "a[2]; if (f=="") l=0}}' <<<"$script_source"
}

### End of user code helpers

test-user-code() {
    vm-command-q "kubectl get pods 2>&1 | grep -q NAME" && vm-command "kubectl delete pods --all --now"
    ( eval "$code" ) || {
        TEST_FAILURES="${TEST_FAILURES} test script failed"
    }
}

# Validate parameters
INTERACTIVE_MODE=0
mode=$1
user_script_file=$2
vm=${vm-"crirm-test-numa"}
cri_resmgr_cfg=${cri_resmgr_cfg-"${SCRIPT_DIR}/cri-resmgr-memtier.cfg"}
cleanup=${cleanup-0}
reinstall_cri_resmgr=${reinstall_cri_resmgr-0}
numanodes=${numanodes-'[
    {"cpu": 2, "mem": "1G", "nodes": 2},
    {"cpu": 2, "mem": "2G", "nodes": 2},
    {"nvmem": "8G",
     "dist": 22,
     "node-dist": {"0": 55, "1": 55, "2": 88, "3": 88}
    }]'}
code=${code-"
CPU=1 create guaranteed # creates pod 0, 1 CPU taken
report cpus
CPU=2 create guaranteed # creates pod 1, 3 CPUs taken
report cpus
CPU=3 create guaranteed # creates pod 2, 6 CPUs taken
report cpus
verify \\
    'bin(cpus[0]).count(\"1\") == 1' \\
    'bin(cpus[1]).count(\"1\") == 2' \\
    'bin(cpus[2]).count(\"1\") == 3' \\
    'bin(cpus[0] | cpus[1] | cpus[2]).count(\"1\") == 6'

n=3 create besteffort   # creates pods 3, 4 and 5
verify \\
    '(cpus[3] | cpus[4] | cpus[5]) & (cpus[0] | cpus[1] | cpus[2]) == 0' || true

delete pods pod2        # deletes pod 2, 3 CPUs taken
n=2 create besteffort   # creates pods 6 and 7
CPU=2 n=2 create guaranteed # creates pod 8 and 9, 7 CPUs taken
verify \\
    'bin(cpus[0] | cpus[1] | cpus[8] | cpus[9]).count(\"1\") == 7'
"}

yaml_in_defaults="CPU=1 MEM=100M ISO=true CPUREQ=1 CPULIM=2 MEMREQ=100M MEMLIM=200M"

if [ "$mode" == "help" ]; then
    if [ "$2" == "defaults" ]; then
        echo "Test input defaults:"
        echo ""
        echo "numanodes=${numanodes}"
        echo ""
        echo "cri_resmgr_cfg=${cri_resmgr_cfg}"
        echo ""
        echo -e "code=\"${code}\""
        echo ""
        echo "The defaults to QOSCLASS.yaml.in variables:"
        echo "    ${yaml_in_defaults}"
    elif [ "$2" == "script" ]; then
        if [ "x$3" == "x" ]; then
            help
        else
            help "$3"
        fi
    elif [ "x$2" == "x" ]; then
        usage
    else
        echo "invalid help page, try:"
        echo "  ./run.sh help"
        echo "  ./run.sh help defaults"
        echo "  ./run.sh help script [FUNCTION|all]"
        exit 1
    fi
    exit 0
elif [ "$mode" == "play" ]; then
    speed=${speed-10}
elif [ "$mode" == "test" ]; then
    PV=
elif [ "$mode" == "interactive" ]; then
    PV=
elif [ "$mode" == "record" ]; then
    record
else
    usage
    error "missing valid MODE"
    exit 1
fi

if [ -n "$user_script_file" ]; then
    if [ ! -f "$user_script_file" ]; then
        error "cannot find test script file \"$user_script_file\""
    fi
    code=$(<"$user_script_file")
fi

# Prepare for test/demo
mkdir -p "$OUTPUT_DIR"
mkdir -p "$COMMAND_OUTPUT_DIR"
rm -f "$COMMAND_OUTPUT_DIR"/0*
( echo x > "$OUTPUT_DIR"/x && rm -f "$OUTPUT_DIR"/x ) || {
    error "output directory outdir=$OUTPUT_DIR is not writable"
}

SUMMARY_FILE="$OUTPUT_DIR/summary.txt"
echo -n "" > "$SUMMARY_FILE" || error "cannot write summary to \"$SUMMARY_FILE\""

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

declare -A kind_count # associative arrays for counting created objects, like kind_count[pod]=1
eval "${yaml_in_defaults}"
if [ "$mode" == "interactive" ]; then
    interactive
else
    # Run test/demo
    TEST_FAILURES=""
    test-user-code
fi

# Save logs
host-command "scp $VM_SSH_USER@$VM_IP:cri-resmgr.output.txt \"$OUTPUT_DIR/\""

# Cleanup
if [ "$cleanup" == "0" ]; then
    echo "The VM, Kubernetes and cri-resmgr are left running. Next steps:"
    vm-print-usage
elif [ "$cleanup" == "1" ]; then
    host-stop-vm "$vm"
    host-delete-vm "$vm"
elif [ "$cleanup" == "2" ]; then
    host-stop-vm "$vm"
fi

# Summarize results
exit_status=0
if [ "$mode" == "test" ]; then
    if [ -n "$TEST_FAILURES" ]; then
        echo "Test verdict: FAIL" >> "$SUMMARY_FILE"
    else
        echo "Test verdict: PASS" >> "$SUMMARY_FILE"
    fi
    cat "$SUMMARY_FILE"
fi
exit $exit_status
