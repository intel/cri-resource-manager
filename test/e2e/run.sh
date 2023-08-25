#!/bin/bash

DEMO_TITLE="Container Runtime End-to-End Testing"
DEFAULT_DISTRO="ubuntu-22.04"

PV='pv -qL'

binsrc=${binsrc-local}

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
DEMO_LIB_DIR=$(realpath "$SCRIPT_DIR/../../demo/lib")
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
    echo "            \"debug\" enables k8scri pipe debugging and"
    echo "                    copies sources of all *_src VARs (see below) to vm."
    echo "            \"interactive\" launches interactive shell"
    echo "            for running test script commands"
    echo "            (see ./run.sh help script [FUNCTION])."
    echo "  SCRIPT:   test script file to run instead of the default test."
    echo ""
    echo "  VARs:"
    echo "    vm:      govm virtual machine name."
    echo "             For non-govm-managed hosts: set VM_IP and VM_SSH_USER, too."
    echo "             'ssh \$VM_SSH_USER@\$VM_IP sudo id' must not require password."
    echo "    containerd_src:"
    echo "             \"/host/path/to/go/project\": replace vm /usr/bin binaries"
    echo "             from /host/path/to/go/project/bin directory."
    echo "             The default is to use vm OS package manager containerd."
    echo "    crio_src:"
    echo "             \"/host/path/to/go/project\": replace vm /usr/bin binaries"
    echo "             from /host/path/to/go/project/bin directory."
    echo "             Must be set if crio is a part of \$k8scri and the vm distro"
    echo "             does not have (or implement installing) cri-o packages."
    echo "    crirm_src:"
    echo "             \"/host/path/to/go/project\": replace vm /usr/local/bin binaries"
    echo "             from /host/path/to/go/project/bin directory."
    echo "             The default is to use the project of these e2e tests."
    echo "    runc_src:"
    echo "             \"/host/path/to/go/project\": replace vm /usr/bin binaries"
    echo "             from /host/path/to/go/project/bin directory."
    echo "    distro_binaries:"
    echo "             0: use the normal binaries built for this host (the default)."
    echo "             1: use binaries cross-built for distros."
    echo "    binsrc:  Where to get cri-resmgr to the vm."
    echo "             \"github\": go get from master and build inside vm."
    echo "             \"local\": (the default) copy from \${crirm_src}/bin, or"
    echo "                      from \${crirm_src}/binaries/\$distro if \$distro_binaries=1."
    echo "             \"packages/<distro>\": use distro packages from this dir"
    echo "    reinstall_<containerd|crio|cri_resmgr|cri_resmgr_agent|runc>:"
    echo "             If 1, stop the daemon (if not runc),"
    echo "             then reinstall and restart it before starting test run."
    echo "             The default is 0."
    echo "             Set containerd_src/crio_src/runc_src to install a local build."
    echo "    reinstall_k8s: if 1, destroy existing k8s cluster and create a new one."
    echo "    reinstall_bootstrap: if 1, run the bootstrap and proxy setup commands."
    echo "                         Only available if VM_IP is set when calling the script."
    echo "    reinstall_all: if 1, set all above reinstall_* options to 1."
    echo "    omit_cri_resmgr: if 1, omit checking/installing/starting cri-resmgr."
    echo "    omit_agent: if 1, omit checking/installing/starting cri-resmgr-agent."
    echo "    outdir:  Save output under given directory."
    echo "             The default is \"${SCRIPT_DIR}/output\"."
    echo "    speed:   Demo play speed."
    echo "             The default is 10 (keypresses per second)."
    echo "    cleanup: Level of cleanup after a test run:"
    echo "             0: leave vm running (the default)"
    echo "             1: delete vm"
    echo "             2: stop vm, but do not delete it."
    echo "  Hook VARs:"
    echo "    on_vm_online: code to be executed when SSH connection to vm works."
    echo "    on_k8s_online: code to be executed when Kubernetes is ready for use."
    echo "    on_verify_fail, on_create_fail: code to be executed in case"
    echo "             verify() or create() fails. Example: go to interactive"
    echo "             mode if a verification fails: on_verify_fail=interactive"
    echo "    on_verify, on_create, on_launch: code to be executed every time"
    echo "             after verify/create/launch function"
    echo "    on_{cri,runc,k8s}_install: code to be executed right after installing"
    echo "             these components."
    echo ""
    echo "  VM configuration VARs: (effective when vm is not already configured)"
    echo "    topology: JSON to override NUMA node list used in tests."
    echo "             See: python3 ${DEMO_LIB_DIR}/topology2qemuopts.py --help"
    echo "    distro:  Linux distribution to be / already installed on vm."
    echo "             Supported values: centos-7, centos-8, debian-10, debian-sid"
    echo "                 fedora, fedora-33, opensuse-tumbleweed,"
    echo "                 opensuse-15.4 (same as opensuse), opensuse-15.5, sles,"
    echo "                 ubuntu-18.04, ubuntu-20.04, ubuntu-22.04"
    echo "             If sles: set VM_SLES_REGCODE=<CODE> to use official packages."
    echo "    cgroups: cgroups version in the VM, v1 or v2. The default is v1."
    echo "             cgroups=v2 is supported only on distro=fedora"
    echo "    k8s:     Kubernetes version to be installed on VM creation"
    echo "             The default is the latest available on selected distro."
    echo "             Example: k8s=1.18.10"
    echo "    k8scri:  The container runtime pipe where kubelet connects to."
    echo "             Options are:"
    echo "             \"cri-resmgr|containerd\" cri-resmgr is a proxy to containerd."
    echo "             \"cri-resmgr|crio\"       cri-resmgr is a proxy to cri-o."
    echo "             \"containerd\"            containerd, no cri-resmgr."
    echo "             \"containerd&cri-resmgr\" containerd, cri-resmgr is an NRI plugin."
    echo "             \"crio\"                  cri-o, no cri-resmgr."
    echo "             \"crio&cri-resmgr\"       cri-o, cri-resmgr is an NRI plugin."
    echo "             The default is \"cri-resmgr|containerd\"."
    echo "    k8scni:  The container network interface plugin to install. Options are:"
    echo "             \"cilium\" (the default), \"flannel\", \"weavenet\"."
    echo "    k8smaster: Name of the existing vm whose cluster this vm will join."
    echo "             If empty (default), this vm forms its own single-node cluster."
    echo "    crio_version: Version of cri-o to try to pull in, if cri-o is"
    echo "                  not being installed from sources."
    echo "    setup_proxies: Setup proxies even if not using govm based VM."
    echo "                   This is only needed if you have set VM_IP and want"
    echo "                   the proxy information set in the target host. By default"
    echo "                   the proxies are not set if VM_IP is set."
    echo ""
    echo "  Test input VARs:"
    echo "    cri_resmgr_cfg: configuration file forced to cri-resmgr."
    echo "    cri_resmgr_extra_args: arguments to be added on cri-resmgr"
    echo "             command line when launched"
    echo "    cri_resmgr_agent_extra_args: arguments to be added on"
    echo "              cri-resmgr-agent command line when launched"
    echo "    use_host_images: if \"1\", export images from the host docker"
    echo "              to vm whenever they are available."
    echo "              The default is 0: always pull images from repositories to vm."
    echo "    vm_files: \"serialized\" associative array of files to be created on vm"
    echo "             associative array syntax:"
    echo "             vm_files['/path/file']=file:/path/on/host"
    echo "                                   ='data:,plain text content'"
    echo "                                   =data:;base64,ZGF0YQ=="
    echo "                                   =dir: (creates only /path/file directory)"
    echo "             vm_files['/etc/motd']='data:,hello world'"
    echo "             How to execute run.sh with serialized array:"
    echo "             vm_files=\$(declare -p vm_files) ./run.sh"
    echo "    code:    Variable that contains test script code to be run"
    echo "             if SCRIPT is not given."
    echo "    py_consts: Python code that runs always before pyexec in SCRIPT."
    echo ""
    echo "Default test input VARs: ./run.sh help defaults"
    echo ""
    echo "Create VM 'foo' that runs k8s 1.20.2 on Debian Sid:"
    echo "vm=foo distro=debian-sid k8s=1.20.2 ./run.sh interactive"
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
    speed=60 out "### Running the test in vm=\"$VM_NAME\"."
    host-create-vm "$vm" "$topology"
    vm-networking
    if [ -z "$VM_IP" ]; then
        error "creating VM failed"
    fi
}

screen-install-cri-resmgr() {
    speed=60 out "### Installing CRI Resource Manager to VM."
    vm-install-cri-resmgr
}

screen-launch-cri-resmgr() {
    speed=60 out "### Launching cri-resmgr with config $cri_resmgr_cfg."
    if [ "${binsrc#packages}" != "$binsrc" ]; then
        launch cri-resmgr-systemd
    else
        launch cri-resmgr
    fi
}

screen-create-singlenode-cluster() {
    speed=60 out "### Setting up single-node Kubernetes cluster."
    speed=60 out "### Container runtime parts: $k8scri"
    vm-create-singlenode-cluster
}

screen-launch-cri-resmgr-agent() {
    speed=60 out "### Launching cri-resmgr-agent."
    speed=60 out "### The agent will make cri-resmgr configurable with ConfigMaps."
    launch cri-resmgr-agent
}

get-py-allowed() {
    topology_dump_file="$OUTPUT_DIR/topology_dump.$VM_NAME"
    res_allowed_file="$OUTPUT_DIR/res_allowed.$VM_NAME"
    if ! [ -f "$topology_dump_file" ]; then
        vm-command "$("$DEMO_LIB_DIR/topology.py" bash_topology_dump)" >/dev/null || {
            command-error "error fetching topology_dump from $VM_NAME"
        }
        echo -e "$COMMAND_OUTPUT" > "$topology_dump_file"
    fi
    get-res-allowed "$res_allowed_file"
    py_allowed="
import re
allowed=$("$DEMO_LIB_DIR/topology.py" -t "$topology_dump_file" -r "$res_allowed_file" res_allowed -o json)
_branch_pod=[(p, d, n, c, t, cpu, pod.rsplit('/', 1)[0])
             for p in allowed
             for d in allowed[p]
             for n in allowed[p][d]
             for c in allowed[p][d][n]
             for t in allowed[p][d][n][c]
             for cpu in allowed[p][d][n][c][t]
             for pod in allowed[p][d][n][c][t][cpu]]
# cpu resources allowed for a pod:
packages, dies, nodes, cores, threads, cpus = {}, {}, {}, {}, {}, {}
# mem resources allowed for a pod:
mems = {}
for p, d, n, c, t, cpu, pod in _branch_pod:
    if c == 'mem': # this _branch_pod entry is about memory
        if not pod in mems:
            mems[pod] = set()
        # topology.py can print memory nodes as children of cpu-ful nodes
        # if distance looks like they are behind the same memory controller.
        # The thread field, however, is the true node who contains the memory.
        mems[pod].add(t)
        continue
    # this _branch_pod entry is about cpu
    if not pod in packages:
        packages[pod] = set()
        dies[pod] = set()
        nodes[pod] = set()
        cores[pod] = set()
        threads[pod] = set()
        cpus[pod] = set()
    packages[pod].add(p)
    dies[pod].add('%s/%s' % (p, d))
    nodes[pod].add(n)
    cores[pod].add('%s/%s' % (n, c))
    threads[pod].add('%s/%s/%s' % (n, c, t))
    cpus[pod].add(cpu)

def disjoint_sets(*sets):
    'set.isdisjoint() for n > 1 sets'
    s = sets[0]
    for next in sets[1:]:
        if not s.isdisjoint(next):
            return False
        s = s.union(next)
    return True

def set_ids(str_ids, chars='[a-z]'):
    num_ids = set()
    for str_id in str_ids:
        if '/' in str_id:
            num_ids.add(tuple(int(re.sub(chars, '', s)) for s in str_id.split('/')))
        else:
            num_ids.add(int(re.sub(chars, '', str_id)))
    return num_ids
package_ids = lambda i: set_ids(i, '[package]')
die_ids = lambda i: set_ids(i, '[packagedie]')
node_ids = lambda i: set_ids(i, '[node]')
core_ids = lambda i: set_ids(i, '[nodecore]')
thread_ids = lambda i: set_ids(i, '[nodecorethread]')
cpu_ids = lambda i: set_ids(i, '[cpu]')
"
}

get-res-allowed() {
    local res_allowed_file="$1"
    local retries=5
    while (( retries > 0 )); do
        # Fetch data and update allowed* variables from the virtual machine
        vm-command "$("$DEMO_LIB_DIR/topology.py" bash_res_allowed 'pod[0-9]*c[0-9]*')" >/dev/null || {
            command-error "error fetching res_allowed from $VM_NAME"
        }
        echo -e "$COMMAND_OUTPUT" > "$res_allowed_file"
        # Validate res_allowed_file. Retry if there is same container
        # name with two different sets of allowed CPUs or
        # memories. This is possible if cpuset.cpus of the cgroup has
        # been just changed and different processes in the same
        # container are just going through the change. Or if there are
        # several pods/containers running with the same name.
        awk -F "[ /]" '{if (pod[$1]!=0 && pod[$1]!=""$3""$4){print "error: ambiguous allowed resources for name "$1; exit(1)};pod[$1]=""$3""$4}' "$res_allowed_file" && return 0
        mv "$res_allowed_file" "$res_allowed_file.retries${retries}"
        echo "    see $res_allowed_file.retries${retries} for more details"
        retries=$(( retries - 1 ))
    done
    error "error: container/process name collision: test environment may need cleanup."
}

get-py-cache() {
    # Fetch current cri-resmgr cache from a virtual machine.
    speed=1000 vm-command "cat \"/var/lib/cri-resmgr/cache\"" >/dev/null 2>&1 || {
        command-error "fetching cache file failed"
    }
    cat > "${OUTPUT_DIR}/cache" <<<"$COMMAND_OUTPUT"
    py_cache="
import json
cache=json.load(open(\"${OUTPUT_DIR}/cache\"))
try:
    allocations=json.loads(cache['PolicyJSON']['allocations'])
except KeyError:
    allocations=None
containers=cache['Containers']
pods=cache['Pods']
for _contid in list(containers.keys()):
    try:
        _cmd = ' '.join(containers[_contid]['Command'])
    except:
        continue # Command may be None
    # Recognize echo podXcY ; sleep inf -type test pods and make them
    # easily accessible: containers['pod0c0'], pods['pod0']
    if 'echo pod' in _cmd and 'sleep inf' in _cmd:
        _contname = _cmd.split()[3] # _contname is podXcY
        _podid = containers[_contid]['PodID']
        _podname = pods[_podid]['Name'] # _podname is podX
        if not allocations is None and _contid in allocations:
            allocations[_contname] = allocations[_contid]
        containers[_contname] = containers[_contid]
        pods[_podname] = pods[_podid]
"
}

resolve-template() {
    local name="$1" r="" d t
    shift
    for d in "$@"; do
        if [ -z "$d" ] || ! [ -d "$d" ]; then
            continue
        fi
        t="$d/$name.in"
        if ! [ -e "$t" ]; then
            continue
        fi
        if [ -z "$r" ]; then
            r="$t"
            echo 1>&2 "template $name resolved to file $r"
        else
            echo 1>&2 "WARNING: template file $r shadows $t"
        fi
    done
    if [ -n "$r" ]; then
        echo "$r"
        return 0
    fi
    return 1
}

is-hooked() {
    local hook_code_var hook_code
    hook_code_var=$1
    hook_code="${!hook_code_var}"
    if [ -n "${hook_code}" ]; then
        return 0 # logic: if is-hooked xyz; then run-hook xyz; fi
    fi
    return 1
}

run-hook() {
    local hook_code_var hook_code
    hook_code_var=$1
    hook_code="${!hook_code_var}"
    echo "Running hook: $hook_code_var"
    eval "${hook_code}"
}

install-files() {
    # Usage: install-files $(declare -p files_assoc_array)
    #
    # Parameter is a serialized associative array with
    # key: target filepath on VM
    # value: source URL ("file:", limited "data:" and "dir:" schemes supported)
    #
    # Example: build an associative array and install files in the array
    #   files['/path/file1']=file:/hostpath/file
    #   files['/path/file2']=data:,hello
    #   files['/path/file3']=data:;base64,aGVsbG8=
    #   files['/path/dir1']='dir:'
    #   install-files "$(declare -p files)"
    local -A files
    eval "files=${1#*=}"
    local tgt src data
    for tgt in "${!files[@]}"; do
        src="${files[$tgt]}"
        case $src in
            "data:,"*)
                data=${src#data:,}
                ;;
            "data:;base64,"*)
                data=$(base64 -d <<< "${src#data:;base64,}")
                ;;
            "file:"*)
                data=$(< "${src#file:}")
                ;;
            "dir:")
                echo -n "Creating on vm: $tgt/... "
                vm-command-q "mkdir -p \"$tgt\"" || {
                    error "failed to make directory to vm \"$tgt\""
                }
                echo "ok."
                continue
                ;;
            *)
                error "invalid source scheme \"${src}\", expected \"data:,\" \"data:;base64,\", \"file:\" or \"dir:\""
                ;;
        esac
        echo -n "Writing on vm: $tgt... "
        vm-write-file "$tgt" "$data" || {
            error "failed to write to vm file \"$tgt\""
        }
        echo "ok."
    done
}

### Test script helpers

install() { # script API
    # Usage: install TARGET
    #
    # Supported TARGETs:
    #   cri-resmgr: install cri-resmgr to VM.
    #               Install latest local build to VM: (the default)
    #                 $ install cri-resmgr
    #               Fetch github master to VM, build and install on VM:
    #                 $ binsrc=github install cri-resmgr
    #   cri-resmgr-webhook: install cri-resmgr-webhook to VM.
    #               Installs from the latest webhook Docker image on the host.
    #
    # Example:
    #   uninstall cri-resmgr
    #   install cri-resmgr
    #   launch cri-resmgr
    local target="$1"
    case "$target" in
        "cri-resmgr")
            vm-install-cri-resmgr
            ;;
        "cri-resmgr-agent")
            vm-install-cri-resmgr-agent
            ;;
        "cri-resmgr-webhook")
            vm-install-cri-resmgr-webhook
            ;;
        *)
            error "unknown target to install \"$1\""
            ;;
    esac
}

uninstall() { # script API
    # Usage: uninstall TARGET
    #
    # Supported TARGETs:
    #   cri-resmgr: stop (kill) cri-resmgr and purge all files from VM.
    #   cri-resmgr-webhook: stop cri-resmgr-webhook and delete webhook files from VM.
    local target="$1"
    case $target in
        "cri-resmgr")
            terminate cri-resmgr
            terminate cri-resmgr-agent
            distro-remove-pkg cri-resource-manager
            vm-command "rm -rf /usr/local/bin/cri-resmgr /usr/bin/cri-resmgr /usr/local/bin/cri-resmgr-agent /usr/bin/cri-resmgr-agent /var/lib/cri-resmgr /etc/cri-resmgr"
            ;;
        "cri-resmgr-agent")
            terminate cri-resmgr-agent
            vm-command "rm -rf /usr/local/bin/cri-resmgr /usr/bin/cri-resmgr /usr/local/bin/cri-resmgr-agent /usr/bin/cri-resmgr-agent /var/lib/cri-resmgr /etc/cri-resmgr"
            ;;
        "cri-resmgr-webhook")
            terminate cri-resmgr-webhook
            vm-command "rm -rf webhook"
            ;;
        *)
            error "uninstall: invalid target \"$target\""
            ;;
    esac
}

launch() { # script API
    # Usage: launch TARGET
    #
    # Supported TARGETs:
    #   cri-resmgr:  launch cri-resmgr on VM. Environment variables:
    #                cri_resmgr_cfg: configuration filepath (on host)
    #                cri_resmgr_extra_args: extra arguments on command line
    #                cri_resmgr_config: "force" (default) or "fallback"
    #                k8scri: if the CRI pipe starts with cri-resmgr
    #                        this launches cri-resmgr as a proxy,
    #                        otherwise as a dynamic NRI plugin.
    #
    #   cri-resmgr-systemd:
    #                launch cri-resmgr on VM using "systemctl start".
    #                Works when installed with binsrc=packages/<distro>.
    #                Environment variables:
    #                cri_resmgr_cfg: configuration filepath (on host)
    #
    #   cri-resmgr-agent:
    #                launch cri-resmgr-agent on VM. Environment variables:
    #                cri_resmgr_agent_extra_args: extra arguments on command line
    #
    #   cri-resmgr-webhook:
    #                deploy cri-resmgr-webhook from the image on VM.
    #
    # Example:
    #   cri_resmgr_cfg=/tmp/topology-aware.cfg launch cri-resmgr
    local target="$1"
    local launch_cmd
    local adjustment_schema="$HOST_PROJECT_DIR/pkg/apis/resmgr/v1alpha1/adjustment-schema.yaml"
    local cri_resmgr_config_option="-${cri_resmgr_config:-force}-config"
    local cri_resmgr_mode=""
    case $target in
        "cri-resmgr")
            host-command "$SCP \"$cri_resmgr_cfg\" $VM_SSH_USER@$VM_IP:" || {
                command-error "copying \"$cri_resmgr_cfg\" to VM failed"
            }
            vm-command "cat $(basename "$cri_resmgr_cfg")"
            if [[ "$k8scri" == cri-resmgr* ]]; then
                # launch cri-resmgr as the top element in the k8s container runtime stack
                cri_resmgr_mode="-relay-socket ${cri_resmgr_sock} -runtime-socket $cri_sock -image-socket $cri_sock"
            else
                # launch cri-resmgr as an NRI plugin to running container runtime
                cri_resmgr_mode="-use-nri-plugin"
            fi
            launch_cmd="cri-resmgr $cri_resmgr_mode $cri_resmgr_config_option $(basename "$cri_resmgr_cfg") $cri_resmgr_extra_args"
            vm-command-q "rm -f $cri_resmgr_pidfile"
            vm-command-q "echo '$launch_cmd' > cri-resmgr.launch.sh ; rm -f cri-resmgr.output.txt"
            vm-command "$launch_cmd  >cri-resmgr.output.txt 2>&1 &"
            vm-wait-process --timeout 30 --pidfile "$cri_resmgr_pidfile" cri-resmgr
            vm-command "grep 'FATAL ERROR' cri-resmgr.output.txt" >/dev/null 2>&1 && {
                command-error "launching cri-resmgr failed with FATAL ERROR"
            }
            vm-command "fuser ${cri_resmgr_pidfile}" >/dev/null 2>&1 || {
                echo "cri-resmgr last output line:"
                vm-command-q "tail -n 1 cri-resmgr.output.txt"
                command-error "launching cri-resmgr failed, cannot find cri-resmgr PID"
            }
            ;;

        "cri-resmgr-agent")
            host-command "$SCP \"$adjustment_schema\" $VM_SSH_USER@$VM_IP:" ||
                command-error "copying \"$adjustment_schema\" to VM failed"
            vm-command "kubectl delete -f $(basename "$adjustment_schema"); kubectl create -f $(basename "$adjustment_schema")"
            launch_cmd="NODE_NAME=\$(hostname) cri-resmgr-agent -kubeconfig /root/.kube/config $cri_resmgr_agent_extra_args"
            vm-command-q "echo '$launch_cmd' >cri-resmgr-agent.launch.sh; rm -f cri-resmgr-agent.output.txt"
            vm-command "$launch_cmd >cri-resmgr-agent.output.txt 2>&1 &"
            vm-wait-process --timeout 30 cri-resmgr-agent
            vm-command "grep 'FATAL ERROR' cri-resmgr-agent.output.txt" >/dev/null 2>&1 &&
                command-error "launching cri-resmgr-agent failed with FATAL ERROR"
            vm-command "fuser ${cri_resmgr_agent_sock}" >/dev/null 2>&1 ||
                command-error "launching cri-resmgr-agent failed, cannot find cri-resmgr-agent PID"
            ;;

        "cri-resmgr-systemd")
            host-command "$SCP \"$cri_resmgr_cfg\" $VM_SSH_USER@$VM_IP:" ||
                command-error "copying \"$cri_resmgr_cfg\" to VM failed"
            vm-command "cp \"$(basename "$cri_resmgr_cfg")\" /etc/cri-resmgr/fallback.cfg"
            vm-command "systemctl daemon-reload ; systemctl start cri-resource-manager" ||
                command-error "systemd failed to start cri-resource-manager"
            vm-wait-process --timeout 30 cri-resmgr
            vm-command "systemctl is-active cri-resource-manager" || {
                vm-command "systemctl status cri-resource-manager"
                command-error "cri-resource-manager did not become active after systemctl start"
            }
            ;;

        "cri-resmgr-webhook")
            kubectl apply -f webhook/webhook-deployment.yaml
            kubectl wait --for=condition=Available -n cri-resmgr deployments/cri-resmgr-webhook ||
                error "cri-resmgr-webhook deployment did not become Available"
            kubectl apply -f webhook/mutating-webhook-config.yaml
            ;;

        *)
            error "launch: invalid target \"$1\""
            ;;
    esac
    is-hooked on_launch && run-hook on_launch
    return 0
}

terminate() { # script API
    # Usage: terminate TARGET
    #
    # Supported TARGETs:
    #   cri-resmgr: stop (kill) cri-resmgr.
    #   cri-resmgr-agent: stop (kill) cri-resmgr-agent.
    #   cri-resmgr-webhook: delete cri-resmgr-webhook from k8s.
    local target="$1"
    case $target in
        "cri-resmgr")
            vm-command "fuser --kill ${cri_resmgr_pidfile} 2>/dev/null"
            ;;
        "cri-resmgr-agent")
            vm-command "fuser --kill ${cri_resmgr_agent_sock} 2>/dev/null"
            ;;
        "cri-resmgr-webhook")
            vm-command "kubectl delete -f webhook/mutating-webhook-config.yaml; kubectl delete -f webhook/webhook-deployment.yaml"
            ;;
        *)
            error "terminate: invalid target \"$target\""
            ;;
    esac
}

sleep() { # script API
    # Usage: sleep PARAMETERS
    #
    # Run sleep PARAMETERS on host.
    host-command "sleep $*"
}

extended-resources() { # script API
    # Usage: extended-resources <add|remove> RESOURCE [VALUE]
    #
    # Examples:
    #   extended-resources remove cmk.intel.com/exclusive-cpus
    #   extended-resources add cmk.intel.com/exclusive-cpus 4
    local action="$1"
    local resource="$2"
    local value="$3"
    local resource_escaped="${resource/\//~1}"
    if [ -z "$resource" ]; then
        error "extended-resource: missing resource"
        return 1
    fi
    # make sure kubectl proxy is running
    vm-command-q "ss -ltn | grep -q 127.0.0.1:8001 || { kubectl proxy &>/dev/null </dev/null & sleep 2 ; }"
    case $action in
        add)
            if [ -z "$value" ]; then
                error "extended-resource: missing value to add to resource $resource"
                return 1
            fi
            vm-command "curl --header 'Content-Type: application/json-patch+json' --request PATCH --data '[{\"op\": \"add\", \"path\": \"/status/capacity/$resource_escaped\", \"value\": \"$value\"}]' http://localhost:8001/api/v1/nodes/\$(hostname)/status"
            ;;
        remove)
            vm-command "curl --header 'Content-Type: application/json-patch+json' --request PATCH --data '[{\"op\": \"remove\", \"path\": \"/status/capacity/$resource_escaped\"}]' http://localhost:8001/api/v1/nodes/\$(hostname)/status"
            ;;
        *)
            error "extended-resource: invalid action \"$action\""
            return 1
            ;;
    esac
}

pyexec() { # script API
    # Usage: pyexec [PYTHONCODE...]
    #
    # Run python3 -c PYTHONCODEs on host. Stops if execution fails.
    #
    # Variables available in PYTHONCODE:
    #   allocations: dictionary: shorthand to cri-resmgr policy allocations
    #                (unmarshaled cache['PolicyJSON']['allocations'])
    #   allowed      tree: {package: {die: {node: {core: {thread: {pod}}}}}}
    #                resource topology and pods allowed to use the resources.
    #   packages, dies, nodes, cores, threads:
    #                dictionaries: {podname: set-of-allowed}
    #                Example: pyexec 'print(dies["pod0c0"])'
    #   cache:       dictionary, cri-resmgr cache
    #
    # Note that variables are *not* updated when pyexec is called.
    # You can update the variables by running "verify" without expressions.
    #
    # Code in environment variable py_consts runs before PYTHONCODE.
    #
    # Example:
    #   verify ; pyexec 'import pprint; pprint.pprint(allowed)'
    PYEXEC_STATE_PY="$OUTPUT_DIR/pyexec_state.py"
    PYEXEC_PY="$OUTPUT_DIR/pyexec.py"
    PYEXEC_LOG="$OUTPUT_DIR/pyexec.output.txt"
    local last_exit_status=0
    {
        echo "import pprint; pp=pprint.pprint"
        echo "# \$py_allowed:"
        echo -e "$py_allowed"
        echo "# \$py_cache:"
        echo -e "$py_cache"
        echo "# \$py_consts:"
        echo -e "$py_consts"
    } > "$PYEXEC_STATE_PY"
    for PYTHONCODE in "$@"; do
        {
            echo "from pyexec_state import *"
            echo -e "$PYTHONCODE"
        } > "$PYEXEC_PY"
        PYTHONPATH="$OUTPUT_DIR:$PYTHONPATH:$DEMO_LIB_DIR" python3 "$PYEXEC_PY" 2>&1 | tee "$PYEXEC_LOG"
        last_exit_status=${PIPESTATUS[0]}
        if [ "$last_exit_status" != "0" ]; then
            error "pyexec: non-zero exit status \"$last_exit_status\", see \"$PYEXEC_PY\" and \"$PYEXEC_LOG\""
        fi
    done
    return "$last_exit_status"
}

pp() { # script API
    # Usage: pp EXPR
    #
    # Pretty-print the value of Python expression EXPR.
    pyexec "pp($*)"
}

report() { # script API
    # Usage: report [VARIABLE...]
    #
    # Updates and reports current value of VARIABLE.
    #
    # Supported VARIABLEs:
    #     allocations
    #     allowed
    #     cache
    #
    # Example: print cri-resmgr policy allocations. In interactive mode
    #          you may use a pager like less.
    #   report allocations | less
    local varname
    for varname in "$@"; do
        if [ "$varname" == "allocations" ]; then
            get-py-cache
            pyexec "
import pprint
pprint.pprint(allocations)
"
        elif [ "$varname" == "allowed" ]; then
            get-py-allowed
            pyexec "
import topology
print(topology.str_tree(allowed))
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
    # Usage: verify [--retry N] [EXPR...]
    #
    # Run python3 -c "assert(EXPR)" to test that every EXPR is True.
    # Stop immediately after the first failing assertion and fail the test.
    #
    # If a verify is expected to fail, failing the whole test can be
    # prevented by running the verify in a subshell (in parenthesis):
    #   (verify 'False') || echo '...this was expected to fail.'
    #
    # --retry N reruns all assertions at most N times before failing
    # the test. All assertions must hold at the same time for a
    # successful verification. By default N=3.
    #
    # Variables available in EXPRs:
    #   See variables in: help pyexec
    #
    # Note that all variables are updated every time verify is called
    # before evaluating (asserting) expressions.
    #
    # Example: require that containers pod0c0 and pod1c0 run on separate NUMA
    #          nodes and that pod0c0 is allowed to run on 4 CPUs:
    #   verify 'set.intersection(nodes["pod0c0"], nodes["pod1c0"]) == set()' \
    #          'len(cpus["pod0c0"]) == 4'
    local retries=3
    local poll_delay=1s
    if [[ "$1" == "--retry" ]]; then
        retries="$2"
        shift; shift
    fi
    while ! _verify "$@"; do
        if (( retries <= 0 )); then
            if is-hooked on_verify_fail; then
                run-hook on_verify_fail
            else
                command-exit-if-not-interactive
            fi
            return 1
        fi
        out "### Retrying verify at most $retries time(s) after $poll_delay..."
        sleep "$poll_delay"
        retries=$(( retries - 1 ))
    done
    is-hooked on_verify && run-hook on_verify
    return 0
}

_verify() {
    get-py-allowed
    get-py-cache
    for py_assertion in "$@"; do
        speed=1000 out "### Verifying assertion '$py_assertion'"
        ( speed=1000 pyexec "
try:
    import time,sys
    assert(${py_assertion})
except KeyError as e:
    print('WARNING: *')
    print('WARNING: *** KeyError - %s' % str(e))
    print('WARNING: *** Your verify expression might have a typo/thinko.')
    print('WARNING: *')
    sys.stdout.flush()
    time.sleep(5)
    raise e
except IndexError as e:
    print('WARNING: *')
    print('WARNING: *** IndexError - %s' % str(e))
    print('WARNING: *** Your verify expression might have a typo/thinko.')
    print('WARNING: *')
    sys.stdout.flush()
    time.sleep(5)
    raise e
" ) || {
                out "### The assertion FAILED
### post-mortem debug help begin ###
cd $OUTPUT_DIR
python3
from pyexec_state import *
$py_assertion
### post-mortem debug help end ###"
                echo "verify: assertion '$py_assertion' failed." >> "$SUMMARY_FILE"
                return 1
        }
        speed=1000 out "### The assertion holds."
    done
    return 0
}

kubectl-force-delete() { # script API
    # Usage: kubectl-force-delete RESOURCE NAME
    #
    # Force-deleting a "Terminating" namespace clears finalizers that
    # have failed to finish. Therefore there may be resources left in the
    # namespace NAME. Following command prints them.
    #
    #     kubectl api-resources --verbs=list --namespaced -o name | \
    #       xargs -n 1 kubectl get --show-kind --ignore-not-found -n NAME
    #
    # Example: delete a namespace that is stuck in the "Terminating" phase
    #
    #     kubectl-force-delete namespace my-namespace

    if [ -z "$1" ]; then
        error "missing RESOURCE"
        return 1
    fi

    if [ -z "$2" ]; then
        error "missing resource NAME"
        return 1
    fi

    if [[ "$1" == "namespace" ]] || [[ "$1" == "ns" ]]; then
        local ns="$2"
        vm-command "
            kubectl get namespace $ns -o json > force-delete-ns.json || exit 0
            (
              grep -E phase.*Terminating force-delete-ns.json || exit 0
              tr -d '\n' < force-delete-ns.json \
              | sed 's/\"finalizers\": \[[^]]\+\]/\"finalizers\": []/' \
              | kubectl replace --raw /api/v1/namespaces/$ns/finalize -f -
            )
            rm -f force-delete-ns.json
            "
    else
        error "unsupported force-delete resource: $1"
        return 1
    fi
}

kubectl() { # script API
    # Usage: kubectl parameters
    #
    # Runs kubectl command on virtual machine.
    vm-command "kubectl $*" || {
        command-error "kubectl $* failed"
    }
}

delete() { # script API
    # Usage: delete PARAMETERS
    #
    # Run "kubectl delete PARAMETERS".
    vm-command "kubectl delete $*" || {
        command-error "kubectl delete failed"
    }
}

instantiate() { # script API
    # Usage: instantiate FILENAME
    #
    # Produces $OUTPUT_DIR/instance/FILENAME. Prints the filename on success.
    # Uses FILENAME.in as source (resolved from $TEST_DIR, $TOPOLOGY_DIR, ...)
    local FILENAME="$1"
    local RESULT="$OUTPUT_DIR/instance/$FILENAME"

    template_file=$(resolve-template "$FILENAME" "$TEST_DIR" "$TOPOLOGY_DIR" "$POLICY_DIR" "$SCRIPT_DIR")
    if [ ! -f "$template_file" ]; then
        error "error instantiating \"$FILENAME\": missing template ${template_file}"
    fi
    mkdir -p "$(dirname "$RESULT")" 2>/dev/null
    eval "echo -e \"$(<"${template_file}")\"" | grep -v '^ *$' > "$RESULT" ||
        error "instantiating \"$FILENAME\" failed"
    echo "$RESULT"
}

declare -a pulled_images_on_vm
create() { # script API
    # Usage: [VAR=VALUE][n=COUNT] create TEMPLATE_NAME
    #
    # Create n instances from TEMPLATE_NAME.yaml.in, copy each of them
    # from host to vm, kubectl create -f them, and wait for them
    # becoming Ready. Templates are searched in $TEST_DIR, $TOPOLOGY_DIR,
    # $POLICY_DIR, and $SCRIPT_DIR in this order of preference. The first
    # template found is used.
    #
    # Parameters:
    #   TEMPLATE_NAME: the name of the template without extension (.yaml.in)
    #
    # Optional parameters (VAR=VALUE):
    #   namespace: namespace to which instances are created
    #   wait: condition to be waited for (see kubectl wait --for=condition=).
    #         If empty (""), skip waiting. The default is wait="Ready".
    #   wait_t: wait timeout. The default is wait_t=240s.
    local template_file
    template_file=$(resolve-template "$1.yaml" "$TEST_DIR" "$TOPOLOGY_DIR" "$POLICY_DIR" "$SCRIPT_DIR")
    local namespace_args
    local template_kind
    template_kind=$(awk '/kind/{print tolower($2)}' < "$template_file")
    local wait=${wait-Ready}
    local wait_t=${wait_t-240s}
    local images
    local image
    local tag
    local errormsg
    local default_name=${NAME:-""}
    if [ -z "$n" ]; then
        local n=1
    fi
    if [ -n "${namespace:-}" ]; then
        namespace_args="-n $namespace"
    else
        namespace_args=""
    fi
    if [ ! -f "$template_file" ]; then
        error "error creating from template \"$template_file.yaml.in\": template file not found"
    fi
    for _ in $(seq 1 $n); do
        kind_count[$template_kind]=$(( ${kind_count[$template_kind]} + 1 ))
        if [ -n "$default_name" ]; then
            local NAME="$default_name"
        else
            local NAME="${template_kind}$(( ${kind_count[$template_kind]} - 1 ))" # the first pod is pod0
        fi
        eval "echo -e \"$(<"${template_file}")\"" | grep -v '^ *$' > "$OUTPUT_DIR/$NAME.yaml"
        host-command "$SCP \"$OUTPUT_DIR/$NAME.yaml\" $VM_SSH_USER@$VM_IP:" || {
            command-error "copying \"$OUTPUT_DIR/$NAME.yaml\" to VM failed"
        }
        vm-command "cat $NAME.yaml"
        images="$(grep -E '^ *image: .*$' "$OUTPUT_DIR/$NAME.yaml" | sed -E 's/^ *image: *([^ ]*)$/\1/g' | sort -u)"
        if [ "${#pulled_images_on_vm[@]}" = "0" ]; then
            # Initialize pulled images available on VM
            vm-command "crictl -i unix://${k8scri_sock} images" >/dev/null &&
            while read -r image tag _; do
                if [ "$image" = "IMAGE" ]; then
                    continue
                fi
                local notopdir_image="${image#*/}"
                local norepo_image="${image##*/}"
                if [ "$tag" = "latest" ]; then
                    pulled_images_on_vm+=("$image")
                    pulled_images_on_vm+=("$notopdir_image")
                    pulled_images_on_vm+=("$norepo_image")
                fi
                pulled_images_on_vm+=("$image:$tag")
                pulled_images_on_vm+=("$notopdir_image:$tag")
                pulled_images_on_vm+=("$norepo_image:$tag")
            done <<< "$COMMAND_OUTPUT"
        fi
        for image in $images; do
            if ! [[ " ${pulled_images_on_vm[*]} " == *" ${image} "* ]]; then
                if [ "$use_host_images" == "1" ] && vm-put-docker-image "$image"; then
                    : # no need to pull the image to vm, it is now imported.
                else
                    vm-command "crictl -i unix://${k8scri_sock} pull \"$image\"" || {
                        errormsg="pulling image \"$image\" for \"$OUTPUT_DIR/$NAME.yaml\" failed."
                        if is-hooked on_create_fail; then
                            echo "$errormsg"
                            run-hook on_create_fail
                        else
                            command-error "$errormsg"
                        fi
                    }
                fi
                pulled_images_on_vm+=("$image")
            fi
        done
        vm-command "kubectl create -f $NAME.yaml $namespace_args" || {
            if is-hooked on_create_fail; then
                echo "kubectl create error"
                run-hook on_create_fail
            else
                command-error "kubectl create error"
            fi
        }
        if [ "x$wait" != "x" ]; then
            speed=1000 vm-command "kubectl wait --timeout=${wait_t} --for=condition=${wait} $namespace_args ${template_kind}/$NAME" >/dev/null 2>&1 || {
                errormsg="waiting for ${template_kind} \"$NAME\" to become ready timed out"
                if is-hooked on_create_fail; then
                    echo "$errormsg"
                    run-hook on_create_fail
                else
                    command-error "$errormsg"
                fi
            }
        fi
    done
    is-hooked on_create && run-hook on_create
    return 0
}

reset() { # script API
    # Usage: reset counters
    #
    # Resets counters
    if [ "$1" == "counters" ]; then
        kind_count[pod]=0
    else
        error "invalid reset \"$1\""
    fi
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
    vm-command-q "kubectl get pods 2>&1 | grep -q NAME" && vm-command "kubectl delete pods --all --now --wait"
    ( eval "$code" ) || {
        TEST_FAILURES="${TEST_FAILURES} test script failed"
    }
}

# Validate parameters
input_var_names="mode user_script_file distro k8scri k8smaster vm cgroups speed binsrc reinstall_all reinstall_containerd reinstall_crio reinstall_cri_resmgr reinstall_k8s reinstall_oneshot outdir cleanup on_verify_fail on_create_fail on_verify on_create on_launch topology cri_resmgr_cfg cri_resmgr_extra_args cri_resmgr_agent_extra_args code py_consts"

INTERACTIVE_MODE=0
mode=$1
user_script_file=$2
distro=${distro:=$DEFAULT_DISTRO}
k8s=${k8s:=}
k8scri=${k8scri:="cri-resmgr|containerd"}
k8smaster=${k8smaster:=}
cri_resmgr_pidfile="/var/run/cri-resmgr*.pid"
cri_resmgr_sock="/var/run/cri-resmgr/cri-resmgr.sock"
cri_resmgr_agent_sock="/var/run/cri-resmgr/cri-resmgr-agent.sock"
case "${k8scri}" in
    "cri-resmgr|containerd")
        k8scri_sock="${cri_resmgr_sock}"
        cri_sock="/var/run/containerd/containerd.sock"
        cri=containerd
        ;;
    "cri-resmgr|crio")
        k8scri_sock="${cri_resmgr_sock}"
        cri_sock="/var/run/crio/crio.sock"
        cri=crio
        ;;
    "containerd")
        k8scri_sock="/var/run/containerd/containerd.sock"
        cri_sock="/var/run/containerd/containerd.sock"
        cri=containerd
        omit_cri_resmgr=1
        omit_agent=1
        ;;
    "containerd&cri-resmgr")
        k8scri_sock="/var/run/containerd/containerd.sock"
        cri_sock="/var/run/containerd/containerd.sock"
        cri=containerd
        ;;
    "crio")
        k8scri_sock="/var/run/crio/crio.sock"
        cri_sock="/var/run/crio/crio.sock"
        cri=crio
        omit_cri_resmgr=1
        omit_agent=1
        ;;
    "crio&cri-resmgr")
        k8scri_sock="/var/run/crio/crio.sock"
        cri_sock="/var/run/crio/crio.sock"
        cri=crio
        ;;
    *)
        error "unsupported k8scri: \"${k8scri}\""
        ;;
esac
distro_binaries=${distro_binaries:=0}
containerd_src=${containerd_src:=}
crio_src=${crio_src:=}
crirm_src=${crirm_src:=$HOST_PROJECT_DIR}
runc_src=${runc_src:=}
crio_version=${crio_version:=}
if [ "$distro_binaries" = "1" ]; then
    if [ -z "$distro" ]; then
        error "distro_binaries=1 but distro is not set"
    fi
    BIN_DIR=${crirm_src}/binaries/$distro
else
    BIN_DIR=${crirm_src}/bin
fi
TOPOLOGY_DIR=${TOPOLOGY_DIR:=e2e}
vm=${vm:=$(basename ${TOPOLOGY_DIR})-${distro}-${cri}}
vm_files=${vm_files:-""}
cgroups=${cgroups:-v1}
cri_resmgr_cfg=${cri_resmgr_cfg:-"${SCRIPT_DIR}/cri-resmgr-topology-aware.cfg"}
cri_resmgr_extra_args=${cri_resmgr_extra_args:-""}
cri_resmgr_agent_extra_args=${cri_resmgr_agent_extra_args:-""}
cleanup=${cleanup:-0}
reinstall_all=${reinstall_all:-0}
reinstall_bootstrap=${reinstall_bootstrap:-0}
reinstall_containerd=${reinstall_containerd:-0}
reinstall_cri_resmgr=${reinstall_cri_resmgr:-0}
reinstall_cri_resmgr_agent=${reinstall_cri_resmgr_agent:-0}
reinstall_crio=${reinstall_crio:-0}
reinstall_k8s=${reinstall_k8s:-0}
reinstall_kubeadm=${reinstall_kubeadm:-0}
reinstall_kubectl=${reinstall_kubectl:-0}
reinstall_kubelet=${reinstall_kubelet:-0}
reinstall_oneshot=${reinstall_oneshot:-0}
reinstall_runc=${reinstall_runc:-0}
if [ "$reinstall_all" == "1" ]; then
    for reinstall_var in ${!reinstall_*}; do
        eval "${reinstall_var}=1"
    done
fi
if [ "$reinstall_k8s" == "1" ]; then
    reinstall_kubeadm=1
    reinstall_kubectl=1
    reinstall_kubelet=1
fi
if [ "$reinstall_bootstrap" == "1" ]; then
    setup_proxies=1
fi
omit_agent=${omit_agent:-0}
omit_cri_resmgr=${omit_cri_resmgr:-0}
use_host_images=${use_host_images:-0}
py_consts="${py_consts:-''}"
topology=${topology:-'[
    {"mem": "1G", "cores": 1, "nodes": 2, "packages": 2, "node-dist": {"4": 28, "5": 28}},
    {"nvmem": "8G", "node-dist": {"5": 28, "0": 17}},
    {"nvmem": "8G", "node-dist": {"2": 17}}
    ]'}
code=${code:-"
CPU=1 create guaranteed # creates pod 0, 1 CPU taken
report allowed
CPU=2 create guaranteed # creates pod 1, 3 CPUs taken
report allowed
CPU=3 create guaranteed # creates pod 2, 6 CPUs taken
report allowed
verify \\
    'len(cpus[\"pod0c0\"]) == 1' \\
    'len(cpus[\"pod1c0\"]) == 2' \\
    'len(cpus[\"pod2c0\"]) == 3' \\
    'len(set.union(cpus[\"pod0c0\"], cpus[\"pod1c0\"], cpus[\"pod2c0\"])) == 6'
n=3 create besteffort   # creates pods 3, 4 and 5
verify \\
    'set.intersection(
       set.union(cpus[\"pod0c0\"], cpus[\"pod1c0\"], cpus[\"pod2c0\"]),
       set.union(cpus[\"pod3c0\"], cpus[\"pod4c0\"], cpus[\"pod5c0\"])) == set()'

delete pods pod2        # deletes pod 2, 3 CPUs taken
n=2 create besteffort   # creates pods 6 and 7
CPU=2 n=2 create guaranteed # creates pod 8 and 9, 7 CPUs taken
verify \\
    'len(set.union(cpus[\"pod0c0\"], cpus[\"pod1c0\"], cpus[\"pod8c0\"], cpus[\"pod9c0\"])) == 7'
"}
warning_delay=${warning_delay:-5}

yaml_in_defaults="CPU=1 MEM=100M ISO=true CPUREQ=1 CPULIM=2 MEMREQ=100M MEMLIM=200M CONTCOUNT=1"

if [ "$mode" == "help" ]; then
    if [ "$2" == "defaults" ]; then
        echo "Test input defaults:"
        echo ""
        echo "topology=${topology}"
        echo "distro=${distro}"
        echo "k8s=${k8s}"
        echo ""
        echo "cri_resmgr_cfg=${cri_resmgr_cfg}"
        echo ""
        echo "cri_resmgr_extra_args=${cri_resmgr_extra_args}"
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
elif [ "$mode" == "debug" ]; then
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

host-require-cmd jq
host-require-cmd pv

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

## Save test inputs and defaults for the record
mkdir -p "$OUTPUT_DIR/input"; rm -f "$OUTPUT_DIR/input/*"
for var in $input_var_names; do
    if [ -n "${!var}" ]; then
        echo -e "${!var}" > "$OUTPUT_DIR/input/${var}.var"
    fi
done

if [ "$binsrc" == "local" ]; then
    if [ "$omit_cri_resmgr" != "1" ]; then
        [ -f "${BIN_DIR}/cri-resmgr" ] || error "missing \"${BIN_DIR}/cri-resmgr\""
    fi
    if [ "$omit_agent" != "1" ]; then
        [ -f "${BIN_DIR}/cri-resmgr-agent" ] || error "missing \"${BIN_DIR}/cri-resmgr-agent\""
    fi
fi

host-get-vm-config "$vm" || host-set-vm-config "$vm" "$distro" "$cri"

if [ -z "$VM_IP" ] || [ -z "$VM_SSH_USER" ]; then
    screen-create-vm
else
    if [ "$setup_proxies" == "1" ]; then
	vm-setup-proxies
    fi

    if [ "$reinstall_bootstrap" == "1" ]; then
	vm-bootstrap
    fi
fi

is-hooked "on_vm_online" && run-hook "on_vm_online"

if [ "$reinstall_oneshot" == "1" ] || ! vm-command-q "[ -f .vm-setup-oneshot ]"; then
    vm-setup-oneshot
    vm-command-q "touch .vm-setup-oneshot"
fi

if [ -n "$vm_files" ]; then
    install-files "$vm_files"
fi

if [ "$reinstall_containerd" == "1" ] || [ "$reinstall_crio" == "1" ] || ! vm-command-q "( type -p containerd || type -p crio ) >/dev/null"; then
    vm-install-cri
    is-hooked on_cri_install && run-hook on_cri_install
fi

# runc is installed as a dependency of containerd and crio.
# If reinstalling runc is explictly wished for, it is safe to do
# only after (re)installing contaienrd/crio. Otherwise
# a custom locally built runc may be overridden from packages.
if [ "$reinstall_runc" == "1" ] || ! vm-command-q "type -p runc >/dev/null"; then
    vm-install-runc
    is-hooked on_runc_install && run-hook on_runc_install
fi

if [ "$reinstall_k8s" == "1" ] || ! vm-command-q "type -p kubelet >/dev/null"; then
    vm-install-k8s
    is-hooked on_k8s_install && run-hook on_k8s_install
fi

if [ "$reinstall_cri_resmgr" == "1" ]; then
    uninstall cri-resmgr
fi

if [ "$reinstall_cri_resmgr_agent" == "1" ]; then
    uninstall cri-resmgr-agent
fi

if [[ "$k8scri" == cri-resmgr* ]] || [ -n "$crirm_src" ]; then
    if [ "$omit_cri_resmgr" != "1" ]; then
        if ! vm-command-q "type -p cri-resmgr >/dev/null"; then
            install cri-resmgr
        fi
    fi

    if [ "$omit_agent" != "1" ]; then
        if ! vm-command-q "type -p cri-resmgr-agent >/dev/null"; then
            install cri-resmgr-agent
        fi
    fi
fi

if [ "$mode" == "debug" ]; then
    vm-command-q "[ -x /root/go/bin/dlv ]" || vm-install-dlv
    if [ -d "$crio_src" ]; then
        vm-dlv-add-src "$crio_src"
    fi
    if [ -d "$containerd_src" ]; then
        vm-dlv-add-src "$containerd_src"
    fi
    if [ -d "$crirm_src" ]; then
        vm-dlv-add-src "$crirm_src"
    fi
    if [ -d "$runc_src" ]; then
        vm-dlv-add-src "$runc_src"
    fi
    echo "How to debug cri-resmgr:"
    echo "- Attach debugger to running cri-resmgr:"
    echo "  ssh $VM_SSH_USER@$VM_IP"
    echo "  sudo /root/go/bin/dlv attach \$(pidof cri-resmgr)"
    echo "- Relaunch cri-resmgr in debugger:"
    echo "  ssh $VM_SSH_USER@$VM_IP"
    echo "  sudo -i"
    echo "  kill -9 \$(pidof cri-resmgr); /root/go/bin/dlv exec /usr/local/bin/cri-resmgr -- -force-config /home/$VM_SSH_USER/*.cfg"
    echo "dlv on VM is ready for use"
    exit 0
fi

if [ -n "$containerd_src" ] && [[ "$k8scri" == *containerd* ]]; then
    vm-check-source-files-changed "$containerd_src" "$containerd_src/bin/containerd"
    vm-check-running-binary "$containerd_src/bin/containerd"
fi

if [ -n "$crio_src" ] && [[ "$k8scri" == *crio* ]]; then
    vm-check-source-files-changed "$crio_src" "$crio_src/bin/crio"
    vm-check-running-binary "$crio_src/bin/crio"
fi

# Start cri-resmgr if not already running
if [ "$omit_cri_resmgr" != "1" ]; then
    if ! vm-command-q "fuser ${cri_resmgr_pidfile}" >/dev/null 2>&1; then
        screen-launch-cri-resmgr
    fi
    if [ -n "$crirm_src" ]; then
        vm-check-source-files-changed "$crirm_src" "$crirm_src/bin/cri-resmgr"
        vm-check-running-binary "$crirm_src/bin/cri-resmgr"
    fi
fi

# Create kubernetes cluster or wait that it is online
if [ "$reinstall_k8s" == "1" ]; then
    vm-destroy-cluster
fi

if vm-command-q "[ ! -f /var/lib/kubelet/config.yaml ]"; then
    if [ -n "$k8smaster" ]; then
        vm-join "$k8smaster"
    else
        screen-create-singlenode-cluster
    fi
else
    # Wait for kube-apiserver to launch (may be down if the VM was just booted)
    vm-wait-process kube-apiserver
fi

# Start cri-resmgr-agent if not already running
if [ "$omit_agent" != "1" ]; then
    if ! vm-command-q "fuser ${cri_resmgr_agent_sock}" >/dev/null; then
        screen-launch-cri-resmgr-agent
    fi
fi

is-hooked "on_k8s_online" && run-hook "on_k8s_online"

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
host-command "$SCP $VM_SSH_USER@$VM_IP:cri-resmgr*.output.txt \"$OUTPUT_DIR/\""

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
