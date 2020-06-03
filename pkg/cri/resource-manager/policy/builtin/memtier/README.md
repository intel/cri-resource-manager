# Memory Tiering

## Overview

The `memtier` policy extends the `topology-aware` policy. It supports
the same features and configuration options, such as `topology hints`
and `annotations`, which the `topology-aware` policy does. Please see
the [documentation for topology-aware
policy](../topology-aware/README.md) for the description of how
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

In order to configure a pod to use a certain memory type, use
`cri-resource-manager.intel.com/memory-type` annotation in the pod spec.
For example, to make a container request both `PMEM` and `DRAM` memory
types, you could use pod metadata such as this:

```
metadata:
  annotations:
    cri-resource-manager.intel.com/memory-type: |
      container1: dram,pmem
```

The `memtier` policy will then aim to allocate resources from a topology
node which can satisfy the memory requirements.

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
    cri-resource-manager.intel.com/memory-type: |
      container1: dram,pmem
    cri-resource-manager.intel.com/cold-start: |
      container1:
        duration: 60s
```

In the above example, `container1` would be initially granted only PMEM
memory controller, but after 60 seconds the DRAM controller would be
added to the container memset.

## Container memory requests and limits

Due to inaccuracies in how `cri-resmgr` calculates memory requests for
pods in QoS class `Burstable`, you should either use `Limit` for setting
the amount of memory for containers in `Burstable` pods or run the
resource-annotating webhook as described in the top-level README file.
