# Podpools Policy

## Overview

The podpools policy implements pod-level workload placement. It
assigns all containers of a pod to the same CPU pool. The number of
CPUs in a pool depends on the type of the pool and is configurable by
user.

## Deployment

### Install cri-resmgr

Deploy cri-resmgr on each node as you would for any other policy. See
[installation](../installation.md) for more details.

## Configuration

The policy is configured using the yaml-based configuration system of CRI-RM.
See [setup and usage](../setup.md#setting-up-cri-resource-manager) for more
details on managing the configuration.

At minimum, you need to specify the active policy in the
configuration, and define at least one pod pool type. For example, the
following configuration dedicates 95 % of non-reserved CPUs on the
node to be used by `dualcpu` pool type. Every pool instance of that
type (`dualcpu[0]`, `dualcpu[1]`, ...) contains two exclusive
CPUs. They are used by containers of pods assigned to the pool, and
those containers cannot use other CPUs.

```yaml
policy:
  Active: podpools
  podpools:
    poolTypes:
      - name: dualcpu
        typeResources:
          cpu: 95 %
        resources:
          cpu: 2
        capacity:
          pod: 1
  ...
```

See the [sample configmap](/sample-configs/podpools-policy.cfg)
for a complete example.

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
pooltype.podpools.cri-resource-manager.intel.com: POOLTYPENAME
```

An example Pod spec for running a workload in a `dualcpu` pool

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: podpools-test
  annotations:
    pooltype.podpools.cri-resource-manager.intel.com: dualcpu
spec:
  containers:
  - name: testcont0
    image: busybox
    command:
      - "sh"
      - "-c"
      - "while :; do grep _allowed_list /proc/self/status; sleep 5; done"
  - name: testcont1
    image: busybox
    command:
      - "sh"
      - "-c"
      - "while :; do grep _allowed_list /proc/self/status; sleep 5; done"
```

If a pod is not annotated, it will be run on shared CPUs. Shared CPUs
are not reserved and do not belong to any pod pool instance.
