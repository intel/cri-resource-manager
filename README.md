# CRI Resource Manager for Kubernetes

## TL;DR Setup

If you want to give CRI Resource Manager a try, here is the list of things
you need to do, assuming you already have a Kubernetes cluster up and running,
using either `containerd` or `cri-o` as the runtime.

  0. [Install](/docs/INSTALL.md) CRI Resource Manager.
  1. Set up kubelet to use CRI Resource Manager as the runtime.
  2. Set up CRI Resource Manager to use the runtime with a policy.

For kubelet you do this by altering its command line options like this:

```
   kubelet <other-kubelet-options> --container-runtime=remote \
     --container-runtime-endpoint=unix:///var/run/cri-resmgr/cri-resmgr.sock
```

For CRI Resource Manager, you need to provide a configuration file, and also a
socket path if you don't use `containerd` or you run it with a different socket
path.

```
   # for containerd with default socket path
   cri-resmgr --force-config <config-file> --runtime-socket unix:///var/run/containerd/containerd.sock
   # for cri-o
   cri-resmgr --force-config <config-file> --runtime-socket unix:///var/run/crio/crio.sock
```

The choice of policy to use along with any potential parameters specific to that
policy are taken from the configuration file. You can take a look at the
[sample configurations](sample-configs) for some minimal/trivial examples. For instance,
you can use [sample-configs/memtier-policy.cfg](sample-configs/memtier-policy.cfg)
as `<config-file>` to activate the topology aware policy with memory tiering support.

**NOTE**: Currently the available policies are work in progress.

### Setting Up kubelet To Use CRI Resource Manager as the Runtime

To let CRI Resource Manager act as a proxy between kubelet and the CRI runtime
you need to configure kubelet to connect to CRI Resource Manager instead of
the runtime. You do this by passing extra command line options to kubelet like
this:

```
   kubelet <other-kubelet-options> --container-runtime=remote \
     --container-runtime-endpoint=unix:///var/run/cri-resmgr/cri-resmgr.sock
```

## Setting Up CRI Resource Manager

Setting up CRI Resource Manager involves pointing it to your runtime and
providing it with a configuration. Pointing to the runtime is done using
the `--runtime-socket <path>` and, optionally, the `--image-socket <path>`.

For providing a configuration there are two options:

  1. use a local configuration YAML file
  2. use the CRI Resource Manager Agent and a `ConfigMap`

The former is easier to set up and it is also the preferred way to run CRI
Resource Manager for development, and in some cases testing. Setting up the
latter is a bit more involved but it allows you to

  - manage policy configuration for your cluster as a single source, and
  - dynamically update that configuration

### Using a Local Configuration From a File

This is the easiest way to run CRI Resource Manager for development or testing.
You can do it with the following command:

```
   cri-resmgr --force-config <config-file> --runtime-socket <path>
```

When started this way CRI Resource Manager reads its configuration from the
given file. It does not fetch external configuration from the node agent and
also disables the config interface for receiving configuration updates.

### Using CRI Resource Manager Agent and a ConfigMap

This setup requires an extra component, the CRI Resource Manager Node Agent,
to monitor and fetch configuration from the ConfigMap and pass it on to CRI
Resource Manager. By default CRI Resource Manager will automatically try to
use the agent to acquire configuration, unless you override this by forcing
a static local configuration using the `--force-config <config-file>` option.
When using the agent, it is also possible to provide an initial fallback for
configuration using the `--fallback-config <config-file>`. This file will be
use before the very first configuration is successfully acquired from the
agent.

Whenever a new configuration is acquired from the agent and successfully
taken into use, this configuration is stored in the cache and will become
the default configuration to take into use the next time CRI Resource
Manager is restarted (unless that time the --force-config option is used).
While CRI Resource Manager is shut down, any cached configuration can be
cleared from the cache using the --reset-config command line option.

See the [later chapter](#cri-resource-manager-node-agent) about how to set
up and configure the agent.


### Changing the Active Policy

Currently CRI Resource Manager will disable changing the active policy using
the agent. That is, once the active policy is recorded in the cache, any
configuration received through the agent that requests a different policy
will be rejected. This limitation will be removed in a future version of
CRI Resource Manager.

However, by default CRI Resource Manager will allow changing policies during
its startup phase. If you want to disable this you can pass the command line
option `--disable-policy-switch` to CRI Resource Manager.

If you run CRI Resource Manager with disabled policy switching, you can still
switch policies by clearing any policy-specific data stored in the cache while
CRI Resource Manager is shut down. You can do this by using the command line
option `--reset-policy`. The whole sequence of switching policies this way is

  - stop cri-resmgr (`systemctl stop cri-resource-manager`)
  - reset policy data (`cri-resmgr --reset-policy`)
  - change policy (`$EDITOR /etc/cri-resource-manager/fallback.cfg`)
  - start cri-resmgr (`systemctl start cri-resource-manager`)

## CRI Resource Manager Mutating Webhook

By default CRI Resource Manager does not see the original container *resource
requirements* specified in the *Pod Spec*. It tries to calculate these for `cpu`
and `memory` *compute resource*s using the related parameters present in the
CRI container creation request. The resulting estimates are normally accurate
for `cpu`, and also for `memory` `limits`. However, it is not possible to use
these parameters to estimate `memory` `request`s or any *extended resource*s.

If you want to make sure that CRI Resource Manager uses the origin *Pod Spec*
*resource requirement*s, you need to duplicate these as *annotations* on the Pod.
This is necessary if you plan using or writing a policy which needs *extended
resource*s.

This process can be fully automated using the [CRI Resource Manager Annotating
Webhook](cmd/cri-resmgr-webhook). Once you built the docker image for it using
the [provided Dockerfile][cmd/cri-resmgr-webhook/Dockerfile] and published it,
you can set up the webhook with these commands:

```
  kubectl apply -f cmd/cri-resmgr-webhook/mutating-webhook-config.yaml
  kubectl apply -f cmd/cri-resmgr-webhook/webhook-deployment.yaml

```


## CRI Resource Manager Node Agent

CRI Resource Manager can be configured dynamically using the CRI Resource
Manager Node Agent and Kubernetes ConfigMaps. The agent can be build using
the [provided Dockerfile](cmd/cri-resmgr-agent/Dockerfile). It can be deployed
as a `DaemonSet` in the cluster using the [provided deployment file](cmd/cri-resmgr-agent/agent-deployment.yaml).

To run the agent manually or as a `systemd` service, set the environment variable
`NODE_NAME` to the name of the cluster node the agent is running on. If necessary
pass it the credentials for accessing the cluster using the the `-kubeconfig <file>`
command line option.

The agent monitors two ConfigMaps for the node, a primary node-specific one, and
a secondary group-specific or default one, depending on whether the node belongs
to a configuration group. The node-specific ConfigMap always takes precedence over
the others.

The names of these ConfigMaps are

1. `cri-resmgr-config.node.$NODE_NAME`: primary, node-specific configuration
2. `cri-resmgr-config.group.$GROUP_NAME`: secondary group-specific node configuration
3. `cri-resmgr-config.default`: secondary: secondary default node configuration

You can assign a node to a configuration group by setting the
`cri-resource-manager.intel.com/group` label on the node to the name of
the configuration group. You can remove a node from its group by deleting the node
group label.

There is a [sample ConfigMap spec](sample-configs/cri-resmgr-configmap.example.yaml)
that contains anode-specific, a group-specific, and a default ConfigMap examples.
See [any available policy-specific documentation](docs) for more information on the
policy configurations.


### Container Adjustments

When the agent is in use, it is also possible to `adjust` container `resource
assignments` externally, using dedicated `Adjustment` `Custom Resources` in
the `adjustments.criresmgr.intel.com` group. You can use the
[provided schema](pkg/apis/resmgr/v1alpha1/adjustment-schema.yaml) to define
the `Adjustment` resource. Then you can copy and modify the
[sample adjustment CR](sample-configs/external-adjustment.yaml) as a starting
point to test some overrides.

An `Adjustment` consists of a
- `scope`:
  - the nodes and containers to which the adjustment applies to
- adjustment data:
  - updated native/compute resources (`cpu`/`memory` `requests` and `limits`)
  - updated `RDT` and/or `Block I/O` class
  - updated top tier (practically now DRAM) memory limit

All adjustment data is optional. An adjustment can choose to set any or all of
them as necessary. The current handling of adjustment update updates the resource
assignments of containers, marks all existing containers as having pending changes
in all controller domains, then triggers a rebalancing in the active policy. This
will cause all containers to be updated.

The scope defines which containers on what nodes the adjustment applies to. Nodes
are currently matched/picked by name, but a trailing wildcard (`*`) is allowed and
matches all nodes with the given prefix in their names.

Containers are matched by expressions. These are exactly the same as the expressions
for defining [affinity scopes](docs/container-affinity.md). A single adjustment can
specify multipe node/container match pairs. An adjustment will apply to all containers
in its scope. If an adjustment/update results in conflicts for some container, that is
at least one container is in the scope of multiple adjustments, the adjustment is
rejected and the whole update ignored.

#### Commands for Declaring, Creating, Deleting, and Examining Adjustments

You can declare the custom resource for adjustments with this command:

```
kubectl apply -f pkg/apis/resmgr/v1alpha1/adjustment-schema.yaml
```

You can then add adjustments with a command like this:

```
kubectl apply -f sample-configs/external-adjustment.yaml
```

You can list existing adjustments with the following command. Use the right
`-n namespace` option according to the namespace you use for the agent, for
the configuration and in your adjustment specifications.

```
kubectl get adjustments.criresmgr.intel.com -n kube-system
```

You can examine the contents of a single adjustments with these commands:

```
kubectl describe adjustments external-adjustment -n kube-system
kubectl get adjustments.criresmgr.intel.com/<adjustment-name> -n kube-system -oyaml
```

Or you can examine the contents of all adjustments like this:

```
kubectl get adjustments.criresmgr.intel.com -n kube-system -oyaml
```

Finally, you can delete and adjustment with commands like these:

```
kubectl delete -f sample-configs/external-adjustment.yaml
kubectl delete adjustments.criresmgr.intel.com/<adjustment-name> -n kube-system
```

The status of adjustment updates is propagated back to the `Adjustment` `Custom Resources`,
more specifically into their `Status` fields. With the help of `jq`, you can easily
examine the status of external adjustments using a command like this:

```
kli@r640-1:~> kubectl get -n kube-system adjustments.criresmgr.intel.com -ojson | jq '.items[].status'
{
  "nodes": {
    "r640-1": {
      "errors": {}
    }
  }
}
{
  "nodes": {
    "r640-1": {
      "errors": {}
    }
  }
}
```

The above response is what you get for adjustments that applied without conflicts or
errors. You can see here that only node *r640-1* is in the scope of both of your
existing adjustments and those applied without errors.

If your adjustments resulted in errors, the output will look something like this:

```
klitkey1@r640-1:~> kubectl get -n kube-system adjustments.criresmgr.intel.com -ojson | jq '.items[].status'
{
  "nodes": {
    "r640-1": {
      "errors": {
        "b71a93523e58cb4ba0310aa225b2e2a329cef895ca4b96fcd9d12b375337ea35": "cache: conflicting adjustments for my-pod-r640-1:my-container: adjustment-1,adjustment-2"
      }
    }
  }
}
{
  "nodes": {
    "r640-1": {
      "errors": {
        "b71a93523e58cb4ba0310aa225b2e2a329cef895ca4b96fcd9d12b375337ea35": "cache: conflicting adjustments for my-pod-r640-1:my-container: adjustment-1,adjustment-2"
      }
    }
  }
}
```

Above you can see that on node *r640-1* the container with `ID`
*b71a93523e58cb4ba0310aa225b2e2a329cef895ca4b96fcd9d12b375337ea35*, or *my-container* of
*my-pod-r640-1*, had a conflict. Moreover you can see that the reason of the conflict is
that the container is in the scope of both *adjustment-1* and *adjustment-2*.

You can now fix those adjustments to resolve/remove the conflict then reapply the
adjustments, and then verify that the conflicts are gone.

```
kli@r640-1:~> $EDITOR adjustment-1.yaml adjustment-2.yaml
kli@r640-1:~> kubectl apply -f adjustment-1.yaml && kubectl apply -f adjustment-1.yaml && sleep 2
kli@r640-1:~> kubectl get -n kube-system adjustments.criresmgr.intel.com -ojson | jq '.items[].status'
{
  "nodes": {
    "r640-1": {
      "errors": {}
    }
  }
}
{
  "nodes": {
    "r640-1": {
      "errors": {}
    }
  }
}
```


## Using CRI Resource Manager as a Message Dumper

You can use CRI Resource Manager to simply inspect all proxied CRI requests and
responses without applying any policy. Run CRI Resource Manager with the
provided [sample configuration](sample-configs/cri-full-message-dump.cfg)
for doing this.


## Using Docker as the Runtime

If you must use `docker` as the runtime then the proxying setup is slightly more
complex. Docker does not natively support the CRI API. Normally kubelet runs an
internal protocol translator, `dockershim` to translate between CRI and the
native docker API. To let CRI Resource Manager effectively proxy between kubelet
and `docker` it needs to actually proxy between kubelet and `dockershim`. For this to
be possible, you need to run two instances of kubelet:

  1. real instance talking to CRI Resource Manager/CRI
  2. dockershim instance, acting as a CRI-docker protocol translator

The real kubelet instance you run as you would normally with any other real CRI
runtime, but you specify the dockershim socket for the CRI Image Service:

```
   kubelet <other-kubelet-options> --container-runtime=remote \
     --container-runtime-endpoint=unix:///var/run/cri-resmgr/cri-resmgr.sock \
     --image-service-endpoint=unix:///var/run/dockershim.sock
```

The dockershim instance you run like this, picking the cgroupfs driver according
to your real kubelet instance's configuration:

```
  kubelet --experimental-dockershim --port 11250 --cgroup-driver {systemd|cgroupfs}

```

## Logging and Debugging

You can control logging and debugging with the `--logger-*` command line options. By
default debug logs are globally disabled. You can turn on full debug logs with the
`--logger-debug '*'` command line option.
