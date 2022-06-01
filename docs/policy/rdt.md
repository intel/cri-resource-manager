# RDT (Intel® Resource Director Technology)

## Background

Intel® RDT provides capabilities for cache and memory allocation and
monitoring. In Linux system the functionality is exposed to the user space via
the [resctrl](https://docs.kernel.org/x86/resctrl.html)
filesystem. Cache and memory allocation in RDT is handled by using resource
control groups. Resource allocation is specified on the group level and each
task (process/thread) is assigned to one group. In the context of CRI Resource
we use the term 'RDT class' instead of 'resource control group'.

CRI Resource Manager supports all available RDT technologies, i.e. L2 and L3
Cache Allocation (CAT) with Code and Data Prioritization (CDP) and Memory
Bandwidth Allocation (MBA) plus Cache Monitoring (CMT) and Memory Bandwidth
Monitoring (MBM).


## Overview

RDT configuration in CRI-RM is class-based. Each container gets assigned to an
RDT class. In turn, all processes of the container will be assigned to the RDT
Classes of Service (CLOS) (under `/sys/fs/resctrl`) corresponding the RDT class. CRI-RM will configure
the CLOSes according to its configuration at startup or whenever the
configuration changes.

CRI-RM maintains a direct mapping between Pod QoS classes and RDT classes. If
RDT is enabled CRI-RM tries to assign containers into an RDT class with a name
matching their Pod QoS class. This default behavior can be overridden with
pod annotations.

## Class Assignment

By default, containers get an RDT class with the same name as its Pod QoS class
(Guaranteed, Burstable or Besteffort). If the RDT class is missing the
container will be assigned to the system root class.

The default behavior can be overridden with pod annotations:

- `rdtclass.cri-resource-manager.intel.com/pod: <class-name>` specifies a
  pod-level default that will be used for all containers of a pod
- `rdtclass.cri-resource-manager.intel.com/container.<container-name>: <class-name>`
   specifies container-specific assignment, taking preference over possible
   pod-level annotation (above)

With pod annotations it is possible to specify RDT classes other than
Guaranteed, Burstable or Besteffort.

The default assignment could also be overridden by a policy but currently none
of the builtin policies do that.

## Configuration

### Operating Modes

The RDT controller supports three operating modes, controlled by
`rdt.options.mode` configuration option.

- Disabled: RDT controller is effectively disabled and containers will not be
  assigned and no monitoring groups will be created. Upon activation of this
  mode all CRI-RM specific control and monitoring groups from the resctrl
  filesystem are removed.
- Discovery: RDT controller detects existing non-CRI-RM specific classes from
  the resctrl filesystem and uses these. The configuration of the discovered
  classes is considered read-only and it will not be altered. Upon activation
  of this mode all CRI-RM specific control groups from the resctrl filesystem
  are removed.
- Full: Full operating mode. The controller manages the configuration of the
  resctrl filesystem according to the rdt class definitions in the CRI-RM
  configuration. This is the default operating mode.

### RDT Classes

The RDT class configuration in CRI-RM is a two-level hierarchy consisting of
partitions and classes. It specifies a set of partitions each having a set of
classes.

#### Partitions

Partitions represent a logical grouping of the underlying classes, each
partition specifying a portion of the available resources (L2/L3/MB) which will
be shared by the classes under it. Partitions guarantee non-overlapping
exclusive cache allocation - i.e. no overlap on the cache ways between
partitions is allowed. However, by technology, MB allocations are not
exclusive. Thus, it is possible to assign all partitions 100% of memory
bandwidth, for example.

#### Classes

Classes represent the actual RDT classes containers are assigned to. In
contrast to partitions, cache allocation between classes under a specific
partition may overlap (and they usually do).

The set of RDT classes can be freely specified, but, it should be ensured that
classes corresponding to the Pod QoS classes are specified. Also, the maximum
number of classes (CLOSes) supported by the underlying hardware must not be
exceeded.

### Example

Below is a config snippet that would allocate 60% of the L3 cache lines
exclusively to the Guarenteed class. The remaining 40% L3 is for Burstable and
Besteffort, Besteffort getting only 50% of this. Guaranteed class gets full
memory bandwidth whereas the other classes are throttled to 50%.

```yaml
rdt:
  # Common options
  options:
    # One of Full, Discovery or Disabled
    mode: Full
    # Set to true to disable creation of monitoring groups
    monitoringDisabled: false
    l3:
      # Make this false if L3 CAT must be available
      optional: true
    mb:
      # Make this false if MBA must be available
      optional: true

  # Configuration of classes
  partitions:
    exclusive:
      # Allocate 60% of all L3 cache to the "exclusive" partition
      l3Allocation: "60%"
      mbAllocation: ["100%"]
      classes:
        Guaranteed:
          # Allocate all of the partitions cache lines to "Guaranteed"
          l3Allocation: "100%"
    shared:
      # Allocate 40% L3 cache IDs to the "shared" partition
      # These will NOT overlap with the cache lines allocated for "exclusive" partition
      l3Allocation: "40%"
      mbAllocation: ["50%"]
      classes:
        Burstable:
          # Allow "Burstable" to use all cache lines of the "shared" partition
          l3Allocation: "100%"
        BestEffort:
          # Allow "Besteffort" to use only half of the L3 cache # lines of the "shared" partition.
          # These will overlap with those used by "Burstable"
          l3Allocation: "50%"
```

The configuration also supports far more fine-grained control, e.g. per
cache-ID configuration (i.e. different sockets having different allocation) and
Code and Data Prioritization (CDP) allowing different cache allocation for code
and data paths. If the hardware details are known, raw bitmasks or bit numbers
("0x1f" or 0-4) can be used instead of percentages in order to be able to
configure cache allocations exactly as required. For detailed description of the RDT configuration format with examples see the
{{ '[goresctrl library documentation](https://github.com/intel/goresctrl/blob/{}/doc/rdt.md)'.format(goresctrl_version) }}

See `rdt` in the [example ConfigMap spec](/sample-configs/cri-resmgr-configmap.example.yaml)
for another example configuration.

### Dynamic Configuration

RDT supports dynamic configuration i.e. the resctrl filesystem is reconfigured
whenever a configuration update e.g. via the [Node Agent](../node-agent.md) is
received. However, the configuration update is rejected if it is incompatible
with the set of currently running containers - e.g. the new config is missing a
class that a running container has been assigned to.
