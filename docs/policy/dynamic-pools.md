# Dynamic-Pools Policy

## Overview

The dynamic-pools policy can put the workload into different dynamic-pools. Each dynamic-pool contains several CPUs and can be resized dynamically in terms of the specific algorithms.

The main idea of the algorithm is: on the premise that the CPUs in each dynamic-pool can meet the requests of the pods in the dynamic-pool, the CPUs are allocated based on the CPU utilization of the workload. Dynamic-pools policy try to keep CPU utilization balanced.

CPUs in dynamic-pools can be configured, for example, by setting min and max frequencies on CPU cores and uncore.

## How It Works

1. The user configures the dynamic-pool types from which the policy instantiates dynamic-pools. In addition to the dynamic-pools configured by the user, there is also a built-in dynamic-pool named shared pool.
2. A dynamic-pool has a set of CPUs and a set of containers running on the CPUs.
3. Every container is assigned to a dynamic-pool. Dynamic-pools policy allows a container to use all CPUs of its pool and no other CPUs.
4. Each logical CPU belongs to exactly one dynamic-pool. There cannot be CPUs that do not belong to any dynamic-pool.
5. The number of CPUs in a dynamic-pool can change. If CPUs are added to a dynamic-pool, then all containers in the dynamic-pool can use more CPUs. The opposite is true if the CPUs are removed.
6. As CPUs are added to or removed from the dynamic-pool, the CPUs are reconfigured according to the dynamic-pool's CPU class attributes or the idle CPU class attributes.
7. Updating the number of CPUs in dynamic-pools:
   - The dynamic-pools policy needs to update the number of CPUs in dynamic-pools when starting policy, creating pods, deleting pods, updating configurations, and at regular intervals.
   - The number of CPUs in the dynamic-pools is determined by the requests of containers and CPU utilization in the dynamic-pools.
   - The number of CPUs allocated in each dynamic-pool is the sum of the requests of the containers in the dynamic pool and the CPUs allocated based on the CPU utilization of the workload.
8. When a new container is created on a Kubernetes node, the policy first decides the type of the dynamic-pool that will run the container. The decision is based on the annotation of the pod, or the namespace if annotations are not given.

## Deployment

### Install cri-resmgr

Deploy cri-resmgr on each node as you would for any other policy. See [installation](https://intel.github.io/cri-resource-manager/stable/docs/installation.html) for more details.

## Configuration

The dynamic-pools policy is configured using the yaml-based configuration system of CRI-RM. See [setup and usage](https://intel.github.io/cri-resource-manager/stable/docs/setup.html#setting-up-cri-resource-manager) for more details on managing the configuration.

### Parameters

Dynamic-pools policy parameters:

* `PinCPU` controls pinning a container to CPUs of its dynamic-pool. The default is  `true`: the container cannot use other CPUs.
* `PinMemory` controls pinning a container to the memories that are closest to the CPUs of its dynamic-pool. The default is `true`: allow using memory only from the closest NUMA nodes. Warning: this may cause kernel to kill workloads due to out-of-memory error when closest NUMA nodes do not have enough memory. In this situation consider switching this option `false`.
* `ReservedPoolNamespaces` is a list of namespaces (wildcards allowed) that are assigned to the special reserved dynamic-pool, that is, will run on reserved CPUs. This always includes the `kube-system` namespace.
* `DynamicPoolTypes` is a list of dynamic-pool type definitions. Each type can be configured with the following parameters:
  - `Name` of the dynamic-pool type. This is used in pod annotations to assign containers to dynamic-pool of this type.
  - `Namespaces` is a list of namespaces (wildcards allowed) whose pods should be assigned to this dynamic-pool type, unless overridden by pod annotations.
  - `CpuClass` specifies the name of the CPU class according to which CPUs of dynamic-pools are configured.
  - `AllocatorPriority` (0: High, 1: Normal, 2: Low, 3: None). CPU allocator parameter, used when creating new or resizing existing dynamic-pools.

Related configuration parameters:

* `policy.ReservedResources.CPU` specifies the (number of) CPUs in the special `reserved` dynamic-pool. By default all containers in the `kube-system` namespace are assigned to the reserved dynamic-pool.
* `policy.AvailableResources.CPU` specifies the CPUs that can be used by the policy, including `policy.ReservedResources.CPU`.
* `cpu.classes` defines CPU classes and their parameters (such as `minFreq`, `maxFreq`, `uncoreMinFreq` and `uncoreMaxFreq`).

### Example

```yaml
cpu:
  classes:
    pool1-cpuclass:
      maxFreq: 1500000
      minFreq: 2000000
    pool2-cpuclass:
      maxFreq: 2000000
      minFreq: 2500000
policy:
  Active: dynamic-pools
  ReservedResources:
      CPU: cpuset:0
  dynamic-pools:
    PinCPU: true
    PinMemory: true
    DynamicPoolTypes:
      - Name: "pool1"
        Namespaces:
          - "pool1"
        CPUClass: "pool1-cpuclass"
      - Name: "pool2"
        Namespaces:
          - "pool2"
        CPUClass: "pool2-cpuclass"
```

### Update Dynamic-Pools at Regular Intervals

The dynamic-pools policy can be set at regular intervals, based on the cpu utilization of the workload in each pool, to update the cpu allocation, and use the `--rebalance-interval` option to set the interval.

### Assigning a Container to a Dynamic-pool

The dynamic-pool type of a container can be defined in pod annotations. In the example below, the first annotation sets the dynamic-pool type (`DPT`) of a single container (`CONTAINER_NAME`). The last two annotations set the default dynamic-pools type for all containers in the pod.

```yaml
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/container.CONTAINER_NAME: DPT
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: DPT
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com: DPT
```

If a pod has no annotations, its namespace is matched to the `namespace` of dynamic-pool types. The first matching dynamic-pool type is used.

If the namespace does not match, the container is assigned to the `shared` dynamic-pool.

## Metrics and Debugging

In order to enable more verbose logging and metrics exporting from the dynamic-pools policy, enable instrumentation and policy debugging from the CRI-RM global config:

```yaml
instrumentation:
  # The dynamic-pools policy exports containers running in each dynamic-pool,
  # and cpusets of dynamic-pools. Accessible in command line:
  # curl --silent http://localhost:8891/metrics
  HTTPEndpoint: :8891
  PrometheusExport: true
logger:
  Debug: policy
```

Use the `--metrics-interval` option to set the interval for updating metrics data.