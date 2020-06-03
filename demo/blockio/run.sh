#!/bin/bash

DEMO_TITLE="CRI Resource Manager: Block I/O Demo"

PV='pv -qL'

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"

LIB_DIR=$SCRIPT_DIR/../lib

source $LIB_DIR/host.bash
source $LIB_DIR/vm.bash

out() {
    speed=${speed-10}
    echo "$1" | $PV $speed
    echo | $PV $speed
}

record() {
    clear
    out "Recording this screencast..."
    host-command "asciinema rec -t \"$DEMO_TITLE\" crirm-demo-blockio.cast -c \"./run.sh play\""
}

error() {
    (echo ""; echo "error: $1" ) >&2
    exit 1
}

screen-create-vm() {
    speed=60 out '### Running the demo in a single-node cluster in a VM.'
    host-create-vm $vm
    vm-networking
    if [ -z "$VM_IP" ]; then
        error "creating VM failed"
    fi
}

screen-install-k8s() {
    speed=60 out "### Installing Kubernetes to the VM."
    vm-command "apt update && apt install -y apt-transport-https curl containerd"
    if [ -n "$http_proxy" ] || [ -n "$https_proxy" ] || [ -n "$no_proxy" ]; then
        speed=120 vm-command "mkdir -p /etc/systemd/system/containerd.service.d; (echo '[Service]'; echo 'Environment=HTTP_PROXY=$http_proxy'; echo 'Environment=HTTPS_PROXY=$https_proxy'; echo \"Environment=NO_PROXY=$no_proxy,$VM_IP,10.96.0.0/12,10.217.0.0/16,\$(hostname)\" ) > /etc/systemd/system/containerd.service.d/proxy.conf; systemctl daemon-reload; systemctl restart containerd"
    fi
    speed=60 vm-command "curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -"
    speed=60 vm-command "echo \"deb https://apt.kubernetes.io/ kubernetes-xenial main\" > /etc/apt/sources.list.d/kubernetes.list"
    vm-command "apt update &&  apt install -y kubelet kubeadm kubectl"
}

screen-install-cri-resmgr() {
    speed=60 out "### Installing CRI Resource Manager to VM."
    vm-command "apt install -y golang make"
    vm-command "go get -d -v github.com/intel/cri-resource-manager"
    CRI_RESMGR_SOURCE_DIR=$(awk '/package.*cri-resource-manager/{print $NF}' < command.output)
    vm-command "cd $CRI_RESMGR_SOURCE_DIR && make install && cd -"
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
    if ! grep -q "initialized successfully" command.output; then
        error "kubeadm init failed"
    fi
    vm-command "mkdir -p \$HOME/.kube"
    vm-command "cp -i /etc/kubernetes/admin.conf \$HOME/.kube/config"
    vm-command "kubectl taint nodes --all node-role.kubernetes.io/master-"
    vm-command "kubectl create -f https://raw.githubusercontent.com/cilium/cilium/v1.6/install/kubernetes/quick-install.yaml"
    if ! vm-command "kubectl rollout status --timeout=360s -n kube-system daemonsets/cilium"; then
        error "installing cilium CNI to Kubernetes timed out"
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
    cmd="pid=\$(ps -A | awk \"/$process/{print \\\$1}\"); echo \$(grep read_bytes /proc/\$pid/io; sleep $measuretime; grep read_bytes /proc/\$pid/io) | awk \"{print \\\"$process read speed: \\\"(\\\$4-\\\$2)/$measuretime/1024\\\" kBps\\\"}\""
    speed=360 outcolor=10 vm-command "$cmd"
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
    vm-wait-process md5sum

    screen-measure-io-speed md5sum

    out "### Reconfiguring cri-resmgr: set SlowReader read speed to 2 MBps."
    out "### This applies to all pods and containers in this block I/O class,"
    out "### both new and already running, like our bb-scanner."
    vm-command "sed -i 's/ThrottleReadBps:.*/ThrottleReadBps: 2Mi/' cri-resmgr-config.default.yaml"
    vm-command "cat cri-resmgr-config.default.yaml"
    vm-command "kubectl apply -f cri-resmgr-config.default.yaml"

    screen-measure-io-speed md5sum

    out "### Thanks for watching!"
    out "### Cleaning up: deleting bb-scanner."
    vm-command "kubectl delete daemonset bb-scanner"
}

if [ "$1" == "play" ]; then
    vm=${vm-"crirm-demo-blockio"}
    speed=${speed-10}

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

    demo-blockio

    speed=120 out "The VM, Kubernetes and cri-resmgr are left running. Next steps:"
    speed=120 out "- Login VM:     ssh $VM_SSH_USER@$VM_IP"
    speed=120 out "- Stop VM:      govm stop $VM_NAME"
    speed=120 out "- Delete VM:    govm delete $VM_NAME"

elif [ "$1" == "record" ]; then
    record
else
    echo "$DEMO_TITLE"
    echo "Usage: [speed=KEYPRESSES_PER_SECOND] [vm=VM_NAME] ./run.sh <play|record>"
    echo "    The default speed is 10."
    echo "    The default vm is \"crirm-demo-blockio\"."
fi
