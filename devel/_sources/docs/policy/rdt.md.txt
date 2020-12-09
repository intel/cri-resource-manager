# RDT (Intel® Resource Director Technology)

## Background

Intel® RDT provides capabilities for cache and memory allocation and
monitoring. In Linux system the functionality is exposed to the user space via
the [resctrl](https://www.kernel.org/doc/Documentation/x86/intel_rdt_ui.txt)
filesystem. Cache and memory allocation in RDT is handled by using resource
control groups. Resource allocation is specified on the group level and each
task (process/thread) is assigned to one group. In the context of CRI Resource
we use the term 'RDT class' instead of 'resource control group'.

CRI Resource Manager supports all available RDT technologies, i.e. L3 Cache
Allocation (CAT) with Code and Data Prioritization (CDP) and Memory Bandwidth
Allocation (MBA) plus Cache Monitoring (CMT) and Memory Bandwidth Monitoring
(MBM).


## Overview

RDT configuration in CRI-RM is class-based. Each container gets assigned to an
RDT class. In turn, all processes of the container will be assigned to the RDT
CLOS  (under `/sys/fs/resctrl`) corresponding the RDT class. CRI-RM will configure
the CLOSes according to its configuration at startup or whenever the
configuration changes.

By default there is a direct mapping between Pod QoS classes and RDT classes:
the containers of the Pod get an RDT class with the same name as its QoS class
(Guaranteed, Burstable or Besteffort). However, that can be overridden with the
rdtclass.cri-resource-manager.intel.com Pod annotation. You can also specify
RDT classes other than Guaranteed, Burstable or Besteffort. In this case, the
Pod can only be assigned to these classes with the Pod annotation, though.
The default behavior can also be overridden by a policy but currently none of
the builtin policies do that.


## Configuration

The RDT configuration in CRI-RM is a two-level hierarchy consisting of
partitions and classes. It specifies a set of partitions each having a set of
classes.

### Partitions

Partitions represent a logical grouping of the underlying classes, each
partition specifying a portion of the available resources (L3/MB) which will be
shared by the classes under it. Partitions guarantee non-overlapping exclusive
cache allocation - i.e. no overlap on the cache ways between partitions is
allowed. However, by technology, MB allocations are not exclusive. Thus, it is
possible to assign all partitions 100% of memory bandwidth, for example.

### Classes

Classes represent the actual RDT classes containers are assigned to. In
contrast to partitions, cache allocation between classes under a specific
partition may overlap (and they usually do).

The set of RDT classes can be freely specified, but, it should be ensured that
classes corresponding to the Pod QoS classes are specified. Also, the maximum
number of classes (CLOSes) supported by the underlying hardware must not be
exceeded.

### Example

Below is a config snippet that would allocate (ca.) 60% of the cache lines
exclusively to the Guarenteed class. The remaining 40% is for Burstable and
Besteffort, Besteffort getting only 50% of this. Guaranteed class gets full
memory bandwidth whereas the other classes are throttled to 50%.

```yaml
metadata:
  name: cri-resmgr-config.default
  namespace: kube-system
data:
...
  rdt: |+
    # Common options
    options:
      l3:
        # Make this false if CAT must be available
        optional: true
      mb:
        # Make this false if MBA must be available
        optional: true
    partitions:
      exclusive:
        # Allocate 60% of all cache IDs to the "exclusive" partition
        l3Allocation: "60%"
        mbAllocation: ["100%"]
        classes:
          Guaranteed:
            # Allocate all of the partitions cache lines to "Guarenteed"
            l3Schema: "100%"
      shared:
        # Allocate 40% of all cache IDs to the "shared" partition
        # These will NOT overlap with the cache lines allocated for "exclusive" partition
        l3Allocation: "40%"
        mbAllocation: ["50%"]
        classes:
          Burstable:
            # Allow "Burstable" to use all cache lines of the "shared" partition
            l3Schema: "100%"
          BestEffort:
            # Allow "Besteffort" to use half of the cache lines of the "shared" partition
            # These will overlap with those used by "Burstable"
            l3Schema: "50%"
```

The configuration also supports far more fine-grained control, e.g. per
cache-ID configuration (i.e. different sockets having different allocation) and
Code and Data Prioritization (CDP) allowing different cache allocation for code
and data paths.

```yaml
...
    partitions:
      exclusive:
        l3Allocation: "60%"
        mbAllocation: ["100%"]
        classes:
          # Automatically gets 100% of what was allocated for the partition
          Guaranteed:
      shared:
        l3Allocation:
          # 'all' denotes the default and must be specified
          all: "40%"
          # Specific cache allotation for cache-ids 2 and 3
          2-3: "20%"
        mbAllocation: ["100%"]
        classes:
          Burstable:
            l3Schema:
              all:
                unified: "100%"
                code: "100%"
                data: "80%"
              mbSchema:
                all: ["80%"]
                2-3: ["50%"]
...
...
```

In addition, if the hardware details are known, raw bitmasks or bit numbers
("0x1f" or 0-4) can be used instead of percentages in order to be able to
configure cache allocations exactly as required. The bits in this case are
corresponding to those in /sys/fs/resctrl/ bitmasks. You can also mix relative
(percentage) and absolute (bitmask) allocations. For cases where the resctrl
filesystem is mounted with `-o mba_MBps` Memory bandwidth must be specifed in
MBps.

```yaml
...
    partitions:
      exclusive:
        # Specify bitmask in bit numbers
        l3Allocation: "8-19"
        # MBps value takes effect when resctrl mount option mba_MBps is used
        mbAllocation: ["100%", "100000MBps"]
        classes:
          # Automatically gets 100% of what was allocated for the partition
          Guaranteed:
      shared:
        # Explicit bitmask
        l3Allocation: "0xff"
        mbAllocation: ["50%", "2000MBps"]
        classes:
          # Burstable gets 100% of what was allocated for the partition
          Burstable:
          BestEffort:
            l3Schema: "50%"
            # Besteffort gets 50% of the 50% (i.e. 25% of total) or 1000MBps
            mbSchema: ["50%", "1000MBps"]
```

See `rdt` in the [example ConfigMap spec](/sample-configs/cri-resmgr-configmap.example.yaml)
for another example configuration.

### Dynamic Configuration

RDT supports dynamic configuration i.e. the resctrl filesystem is reconfigured
whenever a configuration update e.g. via the [Node Agent](../node-agent.md) is
received. However, the configuration update is rejected if it is incompatible
with the set of currently running containers - e.g. the new config is missing a
class that a running container has been assigned to.
