# Quick-start

The following describes the minimum number of steps to get started with CRI
Resource Manager.

## Pre-requisites

- containerd container runtime installed and running
- kubelet installed on your nodes

## Setup CRI-Resmgr

First, install and setup cri-resource-manager.

### Install package

#### CentOS\*, Fedora\*, and SUSE\*

```
CRIRM_VERSION=`curl -s "https://api.github.com/repos/intel/cri-resource-manager/releases/latest" | \
               jq .tag_name | tr -d '"v'`
source /etc/os-release
[ "$ID" = "sles" ] && export ID=opensuse-leap
sudo rpm -Uvh https://github.com/intel/cri-resource-manager/releases/download/v${CRIRM_VERSION}/cri-resource-manager-${CRIRM_VERSION}-0.${ID}-${VERSION_ID}.x86_64.rpm
```

#### Ubuntu\* and Debian\*
```
CRIRM_VERSION=`curl -s "https://api.github.com/repos/intel/cri-resource-manager/releases/latest" | \
               jq .tag_name | tr -d '"v'`
source /etc/os-release
pkg=cri-resource-manager_${CRIRM_VERSION}_${ID}-${VERSION_ID}_amd64.deb; curl -LO https://github.com/intel/cri-resource-manager/releases/download/v${CRIRM_VERSION}/${pkg}; sudo dpkg -i ${pkg}; rm ${pkg}
```


### Setup and verify

Create configuration and start cri-resource-manager
```
sudo cp /etc/cri-resmgr/fallback.cfg.sample /etc/cri-resmgr/fallback.cfg
sudo systemctl enable cri-resource-manager && sudo systemctl start cri-resource-manager
```

See that cri-resource-manager is running
```
systemctl status cri-resource-manager
```

## Kubelet setup

Next, you need to configure kubelet to use cri-resource-manager as it's
container runtime endpoint.

### Existing cluster

When integrating into an existing cluster you need to change kubelet to use
cri-resmgr instead of the existing container runtime (expecting containerd
here).

#### CentOS, Fedora, and SUSE
```
sudo sed '/KUBELET_EXTRA_ARGS/ s!$! --container-runtime-endpoint=/var/run/cri-resmgr/cri-resmgr.sock!' -i /etc/sysconfig/kubelet
sudo systemctl restart kubelet
```

#### Ubuntu and Debian
```
sudo sed '/KUBELET_EXTRA_ARGS/ s!$! --container-runtime-endpoint=/var/run/cri-resmgr/cri-resmgr.sock!' -i /etc/default/kubelet
sudo systemctl restart kubelet
```

### New Cluster

When in the process of setting up a new cluster you simply point the kubelet
to use the cri-resmgr cri sockets on cluster node setup time. Here's an
example with kubeadm:
```
kubeadm join --cri-socket /var/run/cri-resmgr/cri-resmgr.sock \
...

```

## What Next

Congratulations, you now have cri-resource-manager running on your system and
policying container resource allocations. Next, you could see:
- [Installation](installation.md) for more installation options and
  detailed installation instructions
- [Setup](setup.md) for details on setup and usage
- [Node Agent](node-agent.md) for setting up cri-resmgr-agent for dynamic
  configuration and more
- [Webhook](webhook.md) for setting up our resource-annotating webhook
- [Support for Kata Containers\*](setup.md#kata-containers) for setting up
  CRI-RM with Kata Containers
