# Topology-Aware Policy

## Overview

The `topology-aware` builtin policy splits up the node into a tree of pools from
which then resources are allocated to Containers. Currently the tree of pools is
constructed automatically using runtime-discovered hardware topology information
about the node. The pools correspond to the topologically relevant HW components:
sockets, NUMA nodes, and CPUs/cores. The root of the tree corresponds to the full
HW available in the system, the next level corresponds to individual sockets in the
system, the next one to individual NUMA nodes.

The main goal of the `topology-aware` policy is to try and distribute Containers
among the pools (tree nodes) in a way that both maximizes Container performance
and minimizes interference between the Containers of different `Pod`s. This is
accomplished by considering

- topological characteristics of the Container's devices (`topology hints`)
- potential hints provided by the user (in the form of policy-specific `annotations`)
- current availability of hardware resources
- other colocated Containers running on the node

## Features

- aligning workload CPU and memory wrt. the locality of devices used
- exclusive CPU allocation from pools
- discovering and using kernel-isolated CPUs for exclusive allocations
- shared CPU allocation from pools
- mixed (both exclusive and shared) allocation from pools
- exposing the allocated CPU to Containers
- notifying Containers about changes in allocation

## Activating the Topology-Aware Policy

You can activate the tpology-aware policy by setting the `--policy` option of
`cri-resmgr` to `topology-aware`. For instance like this:

```
cri-resmgr --policy topology-aware --reserved-resources cpu=750m
```

## Configuration

### Commandline Options

There are a number of options specific to this policy:

- `--topology-aware-pin-cpu`:
Whether to pin Containers to the CPUs of the assigned pool.

- `--topology-aware-pin-memory`:
Whether to pin Containers to the memory of the assigned pool.

- `--topology-aware-prefer-isolated-cpus:
Whether to try to allocate kernel-isolated CPUs for exclusive usage unless the Pod or Container
is explicitly annotated otherwise.

- `--topology-aware-prefer-shared-cpus`:
Whether to allocate shared CPUs unless the Pod or Container is explicitly annotated otherwise.

### Dynamic Configuration

The `topology-aware` policy can be configured dynamically using the
[`node agent`](/README.md#cri-resource-manager-node-agent). It takes
a JSON configuration with the following keys corresponding to the above
mentioned options:

- `PinCPU`
- `PinMemory`
- `PreferIsolatedCPUs`
- `PreferSharedCPUs`

See the [`documentation`](/README.md#dynamic-configuration) for information about
dynamic configuration.

See the [sample ConfigMap spec](/sample-configs/cri-resmgr-configmap.example.yaml)
for an example which configures the `topology-aware` policy with the built-in
defaults.

### Container / `Pod` Allocation Policy Hints

The `topology-aware` policy recognizes a number of policy-specific annotations
that can be used to provide hints and preferences about how resources should
be allocated to the Containers. These hints are:

- `cri-resource-manager.intel.com/prefer-isolated-cpus`: isolated exclusive CPU preference
- `cri-resource-manager.intel.com/prefer-shared-cpus`: shared allocation preference

#### Isolated Exclusive CPUs

When kernel-isolated CPUs are available ,the `topology-aware` policy will prefer
to allocate those to any Container of a `Pod` in the `Guaranteed QoS class` if
the Container `resource requirements` ask for exactly 1 CPU. If multiple CPUs are
requested, exlusive CPUs will be sliced off from the shared CPU set of the pool.

This default behavior can be changed using the `--topology-aware-prefer-isolated-cpus`
boolean configuration option.

The global default behavior can also be overridden, per Pod or per Container, using
the `cri-resource-manager.intel.com/prefer-isolated-cpus` `annotation`. Setting the
value to `true` asks the policy to prefer isoalted CPUs for exclusive allocation even
if the Container asks for multiple CPUs and only fall back to slicing off shared CPUs
then there is insufficent free isolated capacity. Similarly, setting the value of the
`annotation` to `false` opts out every Container in the `Pod` from taking any isolated
CPUs.

The same mechanism can be used to opt-in or out of isolated CPU usage per Container
within the `Pod` by setting the value of the `annotation` to the string represenation of
a JSON object where each key is the name of a Container and each value is either
`true` or `false`.

#### Shared CPU Allocation

The `topology-aware` policy assumes mixed mode exclusive+shared CPU allocation
preference by default. Under those assumptions every Container of a `Pod` in the
´Guaranteed QoS class` will get exclusive CPUs allocated worth the integer part
of their `CPU request` and a portion of the pool shared CPU set proportional to
the fractional part of their `CPU request`. So for instance, a Container requesting
2.5 CPUs or 2500 milli-CPUs will get by default two exclusive CPUs allocated and
half a CPU worth allocated from the pools CPU set shared with other Container in
the same pool.

This default behavior can be changed using the `--topology-aware-prefer-shared-cpus`
boolean configuration option.

Pods or Containers can opt-out of this assumption using the
`cri-resource-manager.intel.com/prefer-shared-cpus` `annotation`. Setting its value
to `true` will cause the policy to always allocate the entire requested capacity for
all Containers of the Pod from the shared CPUs of a pool. Setting the value to `false`
will cause the policy to allocate any integer portion of the CPU request exclusively
and any fractional part from the shared CPUs.

The same thing can be accomplished per Container by using as value a `JSON object`
similarly to the isolated CPU preference `annotation`: using the Container name as
a key, and `true` or `false` as the value. Moreover, if a negative integer is used
as the value, it is interpreted as `true` with a Container displacement upward in
the tree. For instance, setting the annotation value to

```
  "{\"container-1\": -1, \"container-2\": true}" (or `0` instead of `true`)
```

requests container-1 to be placed to the parent of the pool with the best fitting
score and container-2 to be placed in the best fitting pool itself.

#### Intra-Pod Container Affinity/Anti-affinity

`Containers` within a `Pod` can be annotated with `affinity` or `anti-affinity`
rules, using the `cri-resource-manager.intel.com/affinity` and
`cri-resource-manager.intel.com/anti-affinity` annotations.

`Affinity` indicates a `soft pull` preference while `anti-affinity` indicates
a `soft push` preference. The `topology-aware` policy will try to colocate `containers`
with `affinity` to the same pool and `Containers` with `anti-affinity` to different
pools.

Here is an example snippet of a `Pod Spec` with
  - `container3` having `affinity` to `container1` and `anti-affinity` to `container2`,
  - `container4` having `anti-affinity` to `container2`, and `container3`

```
  annotations:
    cri-resource-manager.intel.com/affinity: |
      container3: [ container1 ]
    cri-resource-manager.intel.com/anti-affinity: |
      container3: [ container2 ]
      container4: [ container2, container3 ]
```

This is actually a shorthand notation for the following, as `key` defaults to
`io.kubernetes.container.name`, and `operator` defaults to `In`.

```
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |+
      container3:
      - match:
          key: io.kubernetes.container.name
          operator: In
          values:
          - container1
    cri-resource-manager.intel.com/anti-affinity: |+
      container3:
      - match:
          key: io.kubernetes.container.name
          operator: In
          values:
          - container2
      container4:
      - match:
          key: io.kubernetes.container.name
          operator: In
          values:
          - container2
          - container3
```

Affinity and anti-affinity can have weights assigned as well. If omitted affinity weights
default to `1` and anti-affinity weights to `-1`. The above example is actually represented
internally with something equivalent to the following.

```
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |+
      container3:
      - match:
          key: io.kubernetes.container.name
          operator: In
          values:
          - container1
        weight: 1
      - match:
          key: io.kubernetes.container.name
          operator: In
          values:
          - container2
        weight: -1
      container4:
      - match:
          key: io.kubernetes.container.name
          operator: In
          values:
          - container2
          - container3
        weight: -1
```

For a more detailed description see [the documentation of annotations](/docs/container-affinity.md).
