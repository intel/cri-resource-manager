#!/bin/bash

TEST_TITLE="CRI validation tests with critest"

PV='pv -qL'

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
DEMO_LIB_DIR=$(realpath "$SCRIPT_DIR/../../demo/lib")
BIN_DIR=$(realpath "$SCRIPT_DIR/../../bin")
OUTPUT_DIR=${outdir-$SCRIPT_DIR/output}
COMMAND_OUTPUT_DIR=$OUTPUT_DIR/commands

# shellcheck disable=SC1091
# shellcheck source=../../demo/lib/command.bash
source "$DEMO_LIB_DIR"/command.bash
# shellcheck disable=SC1091
# shellcheck source=../../demo/lib/host.bash
source "$DEMO_LIB_DIR"/host.bash
# shellcheck disable=SC1091
# shellcheck source=../../demo/lib/vm.bash
source "$DEMO_LIB_DIR"/vm.bash

usage() {
    echo "$TEST_TITLE"
    echo "Usage: [VAR=VALUE] ./run.sh MODE"
    echo "  MODE:     \"play\" plays the test as a demo."
    echo "            \"test\" runs fast, reports pass or fail."
    echo "  VARs:"
    echo "    tests:   space-separated list of cri-resmgr configurations."
    echo "             The default is all *.cfg files in $SCRIPT_DIR."
    echo "    vm:      govm virtual machine name."
    echo "             The default is \"crirm-test-critest\"."
    echo "    speed:   Demo play speed."
    echo "             The default is 10 (keypresses per second)."
    echo "    cleanup: Level of cleanup after a test run:"
    echo "             0: leave VM running (the default)"
    echo "             1: delete VM"
    echo "             2: stop VM, but do not delete it."
    echo "    outdir:  Save output under given directory."
    echo "             The default is \"${SCRIPT_DIR}/output\"."
}

error() {
    (echo ""; echo "error: $1" ) >&2
    exit 1
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

screen-create-vm() {
    speed=60 out "### Running the test in VM \"$vm\"."
    host-create-vm "$vm" "$topology"
    if [ -z "$VM_IP" ]; then
        error "creating VM failed"
    fi
    vm-networking
}

screen-install-containerd() {
    speed=60 out "### Installing Containerd to the VM."
    vm-install-cri
    vm-install-containernetworking
}

screen-copy-cri-resmgr() {
    prefix=/usr/local
    host-command "scp \"$BIN_DIR/cri-resmgr\" \"$SCRIPT_DIR/tsl\" $VM_SSH_USER@$VM_IP:" || {
        command-error "copying cri-resmgr failed"
    }
    vm-command "mv cri-resmgr tsl $prefix/bin/" || {
        command-error "installing cri-resmgr to $prefix/bin failed"
    }
    PV="" vm-command "command -v cri-resmgr" >/dev/null
    ( echo "$COMMAND_OUTPUT" | grep -q $prefix/bin/cri-resmgr ) || {
        command-error "\"cri-resmgr\" does not execute $prefix/bin/cri-resmgr on VM"
    }

}

screen-install-critest() {
    speed=60 out "### Installing critest to VM."
    vm-command "apt update && apt install -y golang make socat"
    vm-command "go get -d github.com/kubernetes-sigs/cri-tools"
    CRI_TOOLS_SOURCE_DIR=$(awk '/package.*cri-tools/{print $NF}' <<< "$COMMAND_OUTPUT")
    [ -n "$CRI_TOOLS_SOURCE_DIR" ] || {
        command-error "downloading cri-tools failed"
    }
    vm-command "pushd \"$CRI_TOOLS_SOURCE_DIR\" && make && make install && popd" || {
        command-error "building and installing cri-tools failed"
    }
}

screen-critest-crirm-config() {
    config_file=$1
    cri_endpoint=/var/run/containerd/containerd.sock
    cri_resmgr_endpoint=/var/run/cri-resmgr/cri-resmgr.sock
    host-command "scp $config_file $VM_SSH_USER@$VM_IP:"
    vm-command "rm -rf *.tsl; killall cri-resmgr; systemctl stop containerd; sleep 1; systemctl start containerd; sleep 1; rm -rf /var/lib/cri-resmgr"
    vm-command "cri-resmgr -force-config $config_file -runtime-socket $cri_endpoint -relay-socket $cri_resmgr_endpoint 2>&1 | tsl -uU -F \"%(ts) s cri-resmgr: %(line)s\" -o cri-resmgr.output.tsl" bg
    sleep 5
    vm-command "critest -runtime-endpoint unix://$cri_resmgr_endpoint 2>&1 | tsl -uU -F \"%(ts) s critest: %(line)s\" -o critest.output.tsl"
    vm-command "killall cri-resmgr"
    vm-command-q "cat *.tsl | sort -n | awk '{if (t_start==0) t_start=\$1; \$1=sprintf(\"%.6fs\", \$1-t_start); print;}'" > "$OUTPUT_DIR/test-$config_file.log"
}

screen-critest-containerd() {
    cri_endpoint=/var/run/containerd/containerd.sock
    vm-command "rm -rf *.tsl; critest -runtime-endpoint unix://$cri_endpoint 2>&1 | tsl -uU -F \"%(ts) s critest: %(line)s\" -o critest.output.tsl"
    vm-command-q "cat *.tsl | sort -n | awk '{if (t_start==0) t_start=\$1; \$1=sprintf(\"%.6fs\", \$1-t_start); print;}'" > "$OUTPUT_DIR/test-containerd.log"
}

require_cmd() {
    cmd=$1
    if ! command -v "$cmd" >/dev/null ; then
        error "required command missing \"${cmd}\", make sure it is in PATH"
    fi
}

# Validate parameters
mode=$1
topology=${topology:='[{"cores": 2, "mem": "8G"}]'}
distro=${distro:="ubuntu-20.04"}
cri=${cri:="containerd"}
vm=${vm:="critest-$distro-$cri"}
cleanup=${cleanup-0}
host-set-vm-config "$vm" "$distro" "$cri"

cd "${SCRIPT_DIR}" || error "failed to cd to \"${SCRIPT_DIR}\""
tests=${tests-*.cfg}

if [ "$mode" == "test" ]; then
    PV=
elif [ "$mode" == "play" ] ; then
    speed=${speed-10}
else
    usage
    error "invalid MODE"
fi

# Prepare for test/demo
mkdir -p "$OUTPUT_DIR"
mkdir -p "$COMMAND_OUTPUT_DIR"
rm -f "$COMMAND_OUTPUT_DIR"/0*
( echo x > "$OUTPUT_DIR/x" && rm -f "$OUTPUT_DIR/x" ) || {
    error "output directory outdir=$OUTPUT_DIR is not writable"
}

if [ -z "$VM_IP" ] || [ -z "$VM_SSH_USER" ] || [ -z "$VM_NAME" ]; then
    screen-create-vm
fi

# always copy new version of the binary to VM
screen-copy-cri-resmgr

if ! vm-command-q "dpkg -l | grep -q containerd"; then
    screen-install-containerd
fi

if ! vm-command-q "command -v critest | grep -q critest"; then
    screen-install-critest
fi

# Run test/demo
# 1. Run critest on cri-resmgr with each config file.
for config_file in $tests; do
    screen-critest-crirm-config "$config_file"
done
# 2. Run critest without cri-resmgr for reference.
screen-critest-containerd

# Cleanup
if [ "$cleanup" == "0" ]; then
    echo "The VM with critest, cri-resmgr and containerd is left running. Next steps:"
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
for testlog in "$OUTPUT_DIR"/test-*.log; do
    {
        echo -n "$(basename "$testlog") "
        awk 'BEGIN{s=0;e=0}/critest: /{if(s==0)s=$1;e=$1}END{printf "(runtime %.2f s): ",e-s}' < "$testlog"
        # remove ansi colors from critest output in the summary
        grep Pending "$testlog" | grep critest: | tail -n 1 | sed -r -e "s/[[:cntrl:]]\[[0-9]+m//g" -e "s/^.* -- //g"
    } >> "$SUMMARY_FILE"
done
exit_status=0
# Declare verdict in test mode
if [ "$mode" == "test" ]; then
    echo "" >> "$SUMMARY_FILE"
    # Test is passed if all critest executions had the same passrate,
    # no matter which cri-resmgr configuration was used.
    if [ "$(awk -F: '/Passed/{print $2}' < "$SUMMARY_FILE" | sort -u | wc -l)" == "1" ]; then
        echo "All critest results are the same." >> "$SUMMARY_FILE"
        echo "Test verdict: PASS" >> "$SUMMARY_FILE"
    else
        echo "Error: critest results are not the same in all configurations." >> "$SUMMARY_FILE"
        echo "Test verdict: FAIL" >> "$SUMMARY_FILE"
        exit_status=1
    fi
fi
echo ""
echo "Summary:"
cat "$SUMMARY_FILE"
exit "$exit_status"
