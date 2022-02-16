# CPU Allocator

CRI Resource Manager has a separate CPU allocator component that helps policies
make educated allocation of CPU cores for workloads.  Currently all policies
except for [static-pools](static-pools.md) utilize the built-in CPU allocator.
See policy specific documentation for more details.

## Topology Based Allocation

The CPU allocator tries to optimize the allocation of CPUs in terms of the
hardware topology. More specifically, it aims at packing all CPUs of one
request "near" each other in order to minimize memory latencies between CPUs.

## CPU Prioritization

The CPU allocator also does automatic CPU prioritization by detecting CPU
features and their configuration parameters. Currently, CRI Resource Manager
supports CPU priority detection based on the `intel_pstate` scaling
driver in the Linux CPUFreq subsystem, and, Intel Speed Select Technology
(SST).

CPUs are divided into three priority classes, i.e. *high*, *normal* and *low*.
Policies utilizing the CPU allocator may choose to prefer certain priority
class for certain types of workloads. For example, prefer (and preserve) high
priority CPUs for high priority workloads.

### Intel Speed Select Technology (SST)

CRI Resource Manager supports detection of all Intel Speed Select Technology
(SST) features, i.e. Speed Select Technology Performance Profile (SST-PP), Base
Frequency (SST-BF), Turbo Frequency (SST-TF) and Core Power (SST-CP).

CPU prioritization is based on detection of the currently active SST features
and their parameterization:

1. If SST-TF has been enabled, all CPUs prioritized by SST-TF are flagged as
   high priority.
1. If SST-CP is enabled but SST-TF disabled, the CPU allocator examines the
   active Classes of Service (CLOSes) and their parameters. CPUs associated
   with the highest priority CLOS will be flagged as high priority, lowest
   priority CLOS will be flagged as low priority and possible "middle priority"
   CLOS as normal priority.
1. If SST-BF has been enabled and SST-TF and SST-CP are inactive, all BF high
   priority cores (having higher guaranteed base frequency) will be flagged
   as high priority.

### Linux CPUFreq

CPUFreq based prioritization only takes effect if Intel Speed Select Technology
(SST) is disabled (or not supported). CRI-RM divides CPU cores into priority
classes based on two parameters:

- base frequency
- EPP (Energy-Performance Preference)

CPU cores with high base frequency (relative to the other cores in the system)
will be flagged as high priority.  Low base frequency will map to low priority,
correspondingly.

CPU cores with high EPP priority (relative to the other cores in the system)
will be marked as high priority cores.
