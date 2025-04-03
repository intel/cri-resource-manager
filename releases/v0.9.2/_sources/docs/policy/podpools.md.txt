# Podpools Policy

## Overview

The podpools policy implements pod-level workload placement. It
assigns all containers of a pod to the same CPU/memory pool. The
number of CPUs in a pool is configurable by user.

## Deployment

### Install cri-resmgr

Deploy cri-resmgr on each node as you would for any other policy. See
[installation](../installation.md) for more details.

## Configuration

The policy is configured using the yaml-based configuration system of CRI-RM.
See [setup and usage](../setup.md#setting-up-cri-resource-manager) for more
details on managing the configuration.

At minimum, you need to specify the active policy in the
configuration, and define at least one pod pool. For example, the
following configuration dedicates 95 % of non-reserved CPUs on the
node to be used by `dualcpu` pools. Every pool instance (`dualcpu[0]`,
`dualcpu[1]`, ...) contains two exclusive CPUs and has a capacity
(`MaxPods`) of one pod. The CPUs are used only by containers of pods
assigned to the pool. Remaining CPUs will be used for running pods
that are not `dualcpu` or `kube-system` pods.

```yaml
policy:
  Active: podpools
  ReservedResources:
    CPU: 1
  podpools:
    Pools:
      - Name: dualcpu
        CPU: 2
        MaxPods: 1
        Instances: 95 %
```

Note that the configuration above allocates two exclusive CPUs for
each pod assigned to the pool. To align with kube-scheduler resource
accounting, requested CPUs of all containers in this kind of pods must
sum up to CPU/MaxPods, that is 2000m CPU in this case.

See the [sample configmap](/sample-configs/podpools-policy.cfg) for a
complete example.

### Debugging

In order to enable more verbose logging for the podpools policy enable
policy debug from the CRI-RM global config:

```yaml
logger:
  Debug: policy
```

## Running Pods in Podpools

The podpools policy assigns a pod to a pod pool instance if the pod
has annotation

```yaml
pool.podpools.cri-resource-manager.intel.com: POOLNAME
```

Following Pod runs in a `dualcpu` pool. This example assumes that
`dualcpu` pools include two CPUs per pod, as in the above
configuration example. Therefore containers in the yaml request 2000m
CPUs in total.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: podpools-test
  annotations:
    pool.podpools.cri-resource-manager.intel.com: dualcpu
spec:
  containers:
  - name: testcont0
    image: busybox
    command:
      - "sh"
      - "-c"
      - "while :; do grep _allowed_list /proc/self/status; sleep 5; done"
    resources:
      requests:
        cpu: 1200m
  - name: testcont1
    image: busybox
    command:
      - "sh"
      - "-c"
      - "while :; do grep _allowed_list /proc/self/status; sleep 5; done"
    resources:
      requests:
        cpu: 800m
```

If a pod is not annotated to run on any specific pod pool and it is
not a `kube-system` pod, it will be run on shared CPUs. Shared CPUs
include left-over CPUs after creating user-defined pools. If all CPUs
were allocated to other pools, reserved CPUs will be used as shared,
too.
