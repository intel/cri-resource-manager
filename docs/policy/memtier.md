# Memtier Policy

## Overview

The `memtier` policy extends the `topology-aware` policy. It supports
the same features and configuration options, such as `topology hints`
and `annotations`, which the `topology-aware` policy does. Please see
the [documentation for topology-aware
policy](topology-aware.md) for the description of how
`topology-aware`policy works and how it is configured.

The main goal of `memtier` policy is to let workloads choose the kinds
of memory it wants to use. The `topology-aware` policy scoring algorithm
for selecting topology nodes is changed so that a workload can belong to
both a CPU node and a memory node in the topology tree -- the CPU
allocation is reserved from the CPU node and the memory controllers are
selected from the memory node. Typically the aim is that the CPU and
memory allocations are done from the same node so that the memory
locality is as good as possible, but the memory allocation may happen
also from a wider pool of memory controllers if the amount of free
memory on a topology node is too low.

## Activation of the Memtier Policy

You can activate the `memtier` policy by setting `--policy` parameter of
`cri-resmgr` to `memtier`. For example:

```
cri-resmgr --policy memtier --reserved-resources cpu=750m
```

## Configuration

The `memtier` policy knows of three kinds of memory: `DRAM`, `PMEM`, and
`HBM`. The various memory types are accessed via separate memory controllers.

  * DRAM (dynamic random-access memory) is regular system main memory.
  * PMEM (persistent memory) is large-capacity memory, such as
    Intel® Optane™ memory.
  * HBM (high-bandwidth memory) is high speed memory, typically found
    on some special-purpose computing systems.

In order to configure a container to use a certain memory type, use the
`memory-type.cri-resource-manager.intel.com` effective annotation in the pod
spec. For example, to make `container1` request both `PMEM` and `DRAM` memory
types, you could use pod metadata such as this:

```
metadata:
  annotations:
    memory-type.cri-resource-manager.intel.com/container.container1: dram,pmem
```

Alternatively, you can use the following deprecated syntax to achieve the same,
but support for this syntax is subject to be dropped in a future release:

```
metadata:
  annotations:
    cri-resource-manager.intel.com/memory-type: |
      container1: dram,pmem
```

The `memtier` policy will then aim to allocate resources from a topology
node which can satisfy the memory requirements.

### Cold Start

The `memtier` policy supports "cold start" functionality. When cold start is
enabled and the workload is allocated to a topology node with both DRAM and
PMEM memory, the initial memory controller is only the PMEM controller. DRAM
controller is added to the workload only after the cold start timeout is
done. The effect of this is that allocated large unused memory areas of
memory don't need to be migrated to PMEM, because it was allocated there to
begin with. Cold start is configured like this in the pod metadata:

```
metadata:
  annotations:
    memory-type.cri-resource-manager.intel.com/container.container1: dram,pmem
    cold-start.cri-resource-manager.intel.com/container.container1: |
      duration: 60s
```

Again, alternatively you can use the following deprecated annotation syntax to
achieve the same, but support for this syntax is subject to be dropped in a
future release:

```
metadata:
  annotations:
    cri-resource-manager.intel.com/memory-type: |
      container1: dram,pmem
    cri-resource-manager.intel.com/cold-start: |
      container1:
        duration: 60s
```

In the above example, `container1` would be initially granted only PMEM
memory controller, but after 60 seconds the DRAM controller would be
added to the container memset.

### Dynamic Page Demotion

The `memtier` policy also supports dynamic page demotion. The idea is to move
rarely-used pages from DRAM to PMEM for those workloads for which both DRAM
and PMEM memory types have been assigned. The configuration for this feature
is done on the memtier policy configuration using three configuration keys:
`DirtyBitScanPeriod`, `PageMovePeriod`, and `PageMoveCount`. All of the three
parameters need to be set to non-zero values in order for the dynamic page
demotion feature to be enabled. See this configuration file fragment as an
example:

```
policy:
  Active: memtier
  memtier:
    DirtyBitScanPeriod: 10s
    PageMovePeriod: 2s
    PageMoveCount: 1000
```

In this setup, every pid in every container in every non-system pod
fulfilling the memory container requirements would have their page ranges
scanned for non-accessed pages every ten seconds. The result of the scan
would be fed to a page-moving loop, which would attempt to move 1000 pages
every two seconds from DRAM to PMEM.

## Container memory requests and limits

Due to inaccuracies in how `cri-resmgr` calculates memory requests for
pods in QoS class `Burstable`, you should either use `Limit` for setting
the amount of memory for containers in `Burstable` pods or run the
resource-annotating webhook as described in the top-level README file.

## Implicit Hardware Topology Hints

`CRI Resource Manager` automatically generates HW `Topology Hints` for
containers before resource allocation by a policy. The `memtier` policy
is hint-aware and takes these hints into account. Since hints indicate
optimal or preferred `HW locality` for devices and potentially local
volumes used by the container, they can alter significantly how resources
are assigned to the container.

Using the 'topologyhints' resource manager annotation key it is possible
to opt out from automatic topology hint generation on a per pod or container
basis.

Use this annotation to opt out a full pod:
```
  annotations:
    topologyhints.cri-resource-manager.intel.com/pod: "false"
```

Use this annotation to opt out container 'foo' in the pod:
```
  annotations:
    topologyhints.cri-resource-manager.intel.com/container.foo: "false"
```

Currently topology hint generation is enabled by default, so using the
annotation as opt in (setting it to "true") should have no effect on the
placement of containers of a pod. This might change in the future however.
