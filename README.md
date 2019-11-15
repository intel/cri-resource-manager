# CRI Resource Manager for Kubernetes

## Introduction

The main purpose of this CRI relay/proxy is to apply various (hardware) resource
allocation policies to containers in a system. The relay sits between the kubelet
and the container runtime, relaying request and responses back and forth between
these two, potentially altering requests as they fly by.

The details of how requests are altered depends on which policy is active inside
the relay. There are several policies available, each geared towards a different
set of goals and implementing different hardware allocation strategies.


## Running the relay as a CRI message dumper

The relay can be run without any real policies activated. This can be useful if
one simply wants to inspect the messages passed over CRI between the kubelet or
any other client using the CRI and the container runtime itself.

For inspecting messages between kubelet and the runtime (or image) services you
need to
1. run dockershim out of the main kubelet process
2. point the relay to the dockershim socket for the runtime (and image) service
3. point the kubelet to the relay socket for the runtime and image service

### Running dockershim

You can use the scripts/testing/dockershim script to start dockershim
separately or to see how this needs to be done. Basically what you need to do
is to pass the kubelet the `--experimental-dockershim` option. For instance:
```
  kubelet --experimental-dockershim --port 11250 --cgroup-driver {systemd|cgroupfs}
```
choosing the cgroup driver according to your system setup.

### Running the CRI relay with no policy, full message dumping

For full message dumping you start the CRI relay like this:
```
  ./cmd/cri-resmgr/cri-resmgr -policy null -dump 'reset,full:.*' -dump-file /tmp/cri.dump
```

### Running kubelet using the proxy as the runtime

You can take a look at the scripts/testing/kubelet script to see how the kubelet
can be started pointing at relay socket for the CRI runtime and image services.
Basically you run kubelet with the same options as you do regularly but pass
also the following extra ones:
```
  --container-runtime=remote \
      --container-runtime-endpoint=unix:///var/run/cri-relay.sock \
      --image-service-endpoint=unix:///var/run/dockershim.sock
```


## Resource-annotating webhook

If you want to test the relay with active policying enabled, you also need to
run a webhook specifically designed to help the policying CRI relay. The webhook
inspects passing Pod creation requests and duplicates the resource requirements
from the pods containers specs as a CRI relay specific annotation.

You can build the webhook docker images with
```
  make images
```

Publish it in a docker registry your cluster can access, edit the webhook
deployment file accordingly in cmd/webhook then configure and deploy it with

```
  kubectl apply -f cmd/webhook/mutating-webhook-config.yaml
  kubectl apply -f cmd/webhook/webhook-deployment.yaml
```

If you want you can try your luck with just updating the deployment file with
the image pointing to your docker registry and see if everything will
automatically get docker built, tagged and published there...

## CRI Resource Manager Node Agent

There is a separate daemon `cri-resmgr-agent` that is expected to be running on
each node alongside `cri-resmgr`. The node agent is responsible for all
communication with the Kubernetes control plane. It has two purposes:
1. Watch for changes in ConfigMap containing the dynamic cri-resmgr
   configuration and relaying any updates to `cri-resmgr`a
2. Relaying any cluster operations (i.e. accesses to the control plane) from
   `cri-resmgr` and its policies to the Kubernetes API server.

The communication between the node agent and the resource manager happens via
gRPC APIs over local unix domain sockets.

When starting the node agent, you need to provide the name of the Kubernetes
Node via an environment variable, as well as a valid kubeconfig.  For example:

```
  NODE_NAME=<my node name> cri-resmgr-agent -kubeconfig <path to kubeconfig>
```


## Running the relay with policies enabled

You can enable active policying of containers by using an appropriate ConfigMap
or a configuration file and setting the `Active` field of the `policy` section
to the desired policy implementation. Note however, that currently you cannot
switch the active policy when you reconfigure cri-resmgr by updating its ConfigMap.

For instance, you can use the following configuration to enable the `static`
policy:

```
policy:
  ReservedResources:
    CPU: 1
  Active: static
```

This will start the relay with the kubelet/CPU Manager-equivalent static policy
enabled and running with 1 CPU reserved for system- and kube- tasks. Similarly,
you can start the relay with the static+ policy using the following configuration:

```
policy:
  ReservedResources:
    CPU: 1
  Active: static-plus
```

The list of available policies can be queried with the `--list-policies`
option.

**NOTE**: The currently available policies are work-in-progress.

## Specifying Configuration

### Static Configuration

cri-resmgr can be configured statically using command line options or
a configuration file. The configuration file accepts the same options,
one option per line, as the command line without leading dashes (-).

For a list of the available command line/configuration file options see
`cri-resmgr -h`.

**NOTE**: some of the policies can be configured with policy-specific
configuration files as well. Those files are different from the one we
refer to here. See to the documentation of the policies themselves for
further details about such potential files and their syntax. The preferred way
for providing these the policy configurations is through Kubernetes
ConfigMap - see the [Dynamic Configuration](#dynamic-configuration) below
for more details.

### Dynamic Configuration

`cri-resmgr` can be configured dynamically using `cri-resmgr-agent`, the
CRI Resource Manager node agent, and Kubernetes ConfigMaps. To run the
agent, set the environment variable NODE_NAME to the name of the node
the agent is running on and pass credentials, if necessary, for accessing
the Kubernetes using the the `-kubeconfig` command line option.

The agent monitors two ConfigMaps for the node, the primary node-specific
ConfigMap and the secondary group-specific or the default one, depending
on whether the node belongs to a configuration group. The node-specific
ConfigMap always takes precedence if it exists. Otherwise the secondary
one is used to configure the node.

The names of these ConfigMaps are

1. cri-resmgr-config.node.$NODE_NAME: primary, node-specific configuration
2. cri-resmgr-config.group.$GROUP_NAME: secondary, group-specific node configuration
3. cri-resmgr-config.default: secondary, default node configuration

You can assign a node to a configuration group by setting the
`cri-resource-manager.intel.com/group` label on the node to the name of
the configuration group. For instance, the command

```
kubectl label --overwrite nodes cl0-slave1 cri-resource-manager.intel.com/group=foo
```

assigns node `cl0-slave1` to the `foo` configuration group.

You can remove a node from its group by deleting the node group label, for
instance like this:

```
kubectl label nodes cl0-slave1 cri-resource-manager.intel.com/group-
```

There is a [sample ConfigMap spec](sample-configs/cri-resmgr-configmap.example.yaml)
that contains a node-specific, a group-specific, and a default sample ConfigMap.
See [any available policy-specific documentation](docs) for more information on the
policy configurations.

## Logging and Debugging

You can control logging and debugging with the `--logger-*` commandline options.
By default logging is globally enabled and debugging is globally disabled. You can
turn on full debugging with the `--logger-debug '*'` commandline option.

