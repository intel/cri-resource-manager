#!/usr/bin/env bash

# fail on unset variables and command errors
#set -eu -o pipefail # -x: is for debugging

# Install actions/runner
if [ ! -d ~/actions-runner ]; then
    mkdir -p ~/actions-runner && cd ~/actions-runner
    curl -so actions-runner-linux-x64-${GHA_RUNNER_VERSION}.tar.gz -L https://github.com/actions/runner/releases/download/v${GHA_RUNNER_VERSION}/actions-runner-linux-x64-${GHA_RUNNER_VERSION}.tar.gz
    tar xzf ./actions-runner-linux-x64-${GHA_RUNNER_VERSION}.tar.gz

    # There is a separate script that configures and executes the self hosted runner so we do not do it here.
fi

export GOPATH="/home/vagrant/go"
mkdir -p $GOPATH/bin
mkdir -p $GOPATH/src

# golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v1.46.2

grep GOPATH ~/.bashrc > /dev/null
if [ $? -ne 0 ]; then
  echo "export GOPATH=$GOPATH" >> ~/.bashrc
  echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
  mkdir -p $GOPATH/bin

  sudo ln -s $GOPATH/bin/govm /usr/local/bin/govm
fi

echo "Setting up govm"
GO111MODULE=off go get -x -d github.com/govm-project/govm && cd $GOPATH/src/github.com/govm-project/govm && go mod tidy && go mod download && go install && cd .. && docker build govm -f govm/Dockerfile -t govm/govm:latest

error() {
    (echo ""; echo "error: $1" ) >&2
    exit 1
}

HOST_VM_IMAGE_DIR="/home/vagrant/vms/images"
mkdir -p "$HOST_VM_IMAGE_DIR" ||
        error "cannot create directory for VM images: $HOST_VM_IMAGE_DIR"

fetch-vm-image() {
    local url="$1"
    local file=$(basename $url)
    local image decompress
    case $file in
        *.xz)
            image=${file%.xz}
            decompress="xz -d"
            ;;
        *.bz2)
            image=${file%.bz2}
            decompress="bzip -d"
            ;;
        *.gz)
            image=${file%.gz}
            decompress="gzip -d"
            ;;
        *)
            image="$file"
            decompress=""
            ;;
    esac
    [ -s "$HOST_VM_IMAGE_DIR/$image" ] || {
        echo "VM image $HOST_VM_IMAGE_DIR/$image not found..."
        [ -s "$HOST_VM_IMAGE_DIR/$file" ] || {
            echo "downloading VM image $image..."
            wget --progress=dot:giga -O "$HOST_VM_IMAGE_DIR/$file" "$url" ||
            echo "failed to download VM image ($url), skipping it"
        }
        if [ -s "$HOST_VM_IMAGE_DIR/$file" ]; then
            if [ -n "$decompress" ]; then
		echo "decompressing VM image $file..."
		( cd "$HOST_VM_IMAGE_DIR" && $decompress $file ) ||
                    error "failed to decompress $file to $image using $decompress"
            fi
            if [ ! -s "$HOST_VM_IMAGE_DIR/$image" ]; then
		error "internal error, fetching+decompressing $url did not produce $HOST_VM_IMAGE_DIR/$image"
            fi
	fi
    }
}


# Pre-install VMs that are needed when the e2e test scripts create govm based VMs and which are run by the github actions.
fetch-vm-image "https://mirrors.xtom.de/fedora/releases/36/Cloud/x86_64/images/Fedora-Cloud-Base-36-1.5.x86_64.qcow2"
fetch-vm-image "https://mirrors.xtom.de/fedora/releases/35/Cloud/x86_64/images/Fedora-Cloud-Base-35-1.2.x86_64.qcow2"
fetch-vm-image "https://mirrors.xtom.de/fedora/releases/34/Cloud/x86_64/images/Fedora-Cloud-Base-34-1.2.x86_64.qcow2"
fetch-vm-image "https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img"
fetch-vm-image "https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
fetch-vm-image "https://cloud-images.ubuntu.com/releases/jammy/release/ubuntu-22.04-server-cloudimg-amd64.img"
fetch-vm-image "https://cloud.debian.org/images/cloud/buster/20200803-347/debian-10-generic-amd64-20200803-347.qcow2"
fetch-vm-image "https://cloud.debian.org/images/cloud/bullseye/daily/latest/debian-11-generic-amd64-daily.qcow2"
fetch-vm-image "https://cloud.debian.org/images/cloud/sid/daily/latest/debian-sid-generic-amd64-daily.qcow2"
fetch-vm-image "https://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud-2003.qcow2.xz"
fetch-vm-image "https://cloud.centos.org/centos/8/x86_64/images/CentOS-8-GenericCloud-8.2.2004-20200611.2.x86_64.qcow2"
fetch-vm-image "https://download.opensuse.org/pub/opensuse/distribution/leap/15.3/appliances/openSUSE-Leap-15.3-JeOS.x86_64-15.3-OpenStack-Cloud-Current.qcow2"
fetch-vm-image "https://download.opensuse.org/repositories/Cloud:/Images:/Leap_15.2/images/openSUSE-Leap-15.2-OpenStack.x86_64-0.0.4-Build8.25.qcow2"
fetch-vm-image "https://ftp.uni-erlangen.de/opensuse/tumbleweed/appliances/openSUSE-Tumbleweed-JeOS.x86_64-OpenStack-Cloud.qcow2"
