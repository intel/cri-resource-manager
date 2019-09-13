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

You can enable active policying of containers by passing the
`--policy <policy-name>` commandline option to the relay. For instance,
```
  ./cmd/cri-resmgr/cri-resmgr --policy static --reserved-resources cpu=1000m
```
will start the relay with the kubelet/CPU Manager-equivalent static policy
enabled and running with 1 CPU reserved for system- and kube- tasks. Similarly,
you can start the relay with the static+ policy using the following command:

```
  ./cmd/cri-resmgr/cri-resmgr --policy static-plus --reserved-resources cpu=1000m
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
CRI Resource Manager node agent, and Kubernetes ConfigMaps. Simply start
`cri-resmgr-agent` providing credentials for accessing the Kubernetes
cluster if necessary using the `-kubeconfig` command line option. It
will monitor the default ConfigMap (`cri-resmgr-config` and send update
notification to `cri-resmgr` whenever the ConfigMap changes.

Currently, only policy-specific configuration is supported via this method.
Each policy has its config under a separate key in the ConfigMap with
further support for Node-specific configuration overrides. ConfigMap keys for
policy configuration adhere the following scheme:
`policy.<policy name>[.<node name>]`'.

The data format of the policy configuration is policy-specific and may be
different between policies (the `static` and `stp` policies use YAML).
There is a
[sample ConfigMap spec](sample-configs/cri-resmgr-configmap.example.yaml)
that contains referential policy configuration for the static and stp policies.
See the policy-specific documentation for more information on the policy
configurations ([STP policy documentation](docs/policy-static-pools.md))



**Tips:**
You can easily populate the default `cri-resmgr-agent` ConfigMap from a
local directory like this:

```
[root@cl0-slave1 tests]# for i in test-configs/static-test/*; do echo "$i:"; cat $i | sed -E 's/^(.)/    \1/g'; done
test-configs/static-test/policy.static:
    RelaxedIsolation: true
[root@cl0-slave1 tests]# kubectl create configmap --namespace kube-system cri-resmgr-config --from-file test-configs/static-test/
configmap/cri-resmgr-config created
[root@cl0-slave1 tests]# kubectl get configmap --namespace kube-system cri-resmgr-config -oyaml
apiVersion: v1
data:
  policy.static: |
    RelaxedIsolation: true
kind: ConfigMap
metadata:
  creationTimestamp: "2019-08-21T18:55:14Z"
  name: cri-resmgr-config
  namespace: kube-system
  resourceVersion: "23689355"
  selfLink: /api/v1/namespaces/kube-system/configmaps/cri-resmgr-config
  uid: 359be72d-c445-11e9-bd86-000001000001
```

You can easily update the default `cri-resmgr-agent` ConfigMap from a local
directory like this:

```
[root@cl0-slave1 tests]# for i in test-configs/static-test/*; do echo "$i:"; cat $i | sed -E 's/^(.)/    \1/g'; done
test-configs/static-test/policy.static:
    RelaxedIsolation: false
[root@cl0-slave1 tests]# kubectl create configmap --namespace kube-system cri-resmgr-config --from-file test-configs/static-test/ --dry-run -oyaml | kubectl replace -f -
configmap/cri-resmgr-config replaced
[root@cl0-slave1 tests]# kubectl get configmap --namespace kube-system cri-resmgr-config -oyaml
apiVersion: v1
data:
  policy.static: |
    RelaxedIsolation: false
kind: ConfigMap
metadata:
  creationTimestamp: "2019-08-21T18:55:14Z"
  name: cri-resmgr-config
  namespace: kube-system
  resourceVersion: "23689639"
  selfLink: /api/v1/namespaces/kube-system/configmaps/cri-resmgr-config
  uid: 359be72d-c445-11e9-bd86-000001000001
```

## Logging and Debugging

You can control logging and debugging with the `--logger-*` commandline options.
By default logging is globally enabled and debugging is globally disabled. You can
turn on full debugging with the `--logger-debug '*'` commandline option.

