# Balloons Policy

## Overview

The balloons policy implements workload placement into "balloons" that
are disjoint CPU pools. Balloons can be inflated and deflated, that is
CPUs added and removed, based on the CPU resource requests of
containers. Balloons can be static or dynamically created and
destroyed. CPUs in balloons can be configured, for example, by setting
min and max frequencies on CPU cores and uncore.

## How It Works

1. User configures balloon types from which the policy instantiates
   balloons.

2. A balloon has a set of CPUs and a set of containers that run on the
   CPUs.

3. Every container is assigned to exactly one balloon. A container is
   allowed to use all CPUs of its balloon and no other CPUs.

4. Every logical CPU belongs to at most one balloon. There can be CPUs
   that do not belong to any balloon.

5. The number of CPUs in a balloon can change during the lifetime of
   the balloon. If a balloon inflates, that is CPUs are added to it,
   all containers in the balloon are allowed to use more CPUs. If a
   balloon deflates, the opposite is true.

6. When a new container is created on a Kubernetes node, the policy
   first decides the type of the balloon that will run the
   container. The decision is based on annotations of the pod, or the
   namespace if annotations are not given.

7. Next the policy decides which balloon of the decided type will run
   the container. Options are:
   - an existing balloon that already has enough CPUs to run its
     current and new containers
   - an existing balloon that can be inflated to fit its current and
     new containers
   - new balloon.

9. When a CPU is added to a balloon or removed from it, the CPU is
   reconfigured based on balloon's CPU class attributes, or idle CPU
   class attributes.

## Deployment

### Install cri-resmgr

Deploy cri-resmgr on each node as you would for any other policy. See
[installation](../installation.md) for more details.

## Configuration

The balloons policy is configured using the yaml-based configuration
system of CRI-RM. See [setup and
usage](../setup.md#setting-up-cri-resource-manager) for more details
on managing the configuration.

### Parameters

Balloons policy parameters:

- `PinCPU` controls pinning a container to CPUs of its balloon. The
  default is `true`: the container cannot use other CPUs.
- `PinMemory` controls pinning a container to the memories that are
  closest to the CPUs of its balloon. The default is `true`: allow
  using memory only from the closest NUMA nodes. Warning: this may
  cause kernel to kill workloads due to out-of-memory error when
  closest NUMA nodes do not have enough memory. In this situation
  consider switching this option `false`.
- `IdleCPUClass` specifies the CPU class of those CPUs that do not
  belong to any balloon.
- `ReservedPoolNamespaces` is a list of namespaces (wildcards allowed)
  that are assigned to the special reserved balloon, that is, will run
  on reserved CPUs. This always includes the `kube-system` namespace.
- `AllocatorTopologyBalancing` affects selecting CPUs for new
  balloons. If `true`, new balloons are created using CPUs on
  NUMA/die/package with most free CPUs, that is, balloons are spread
  across the hardware topology. This helps inflating balloons within
  the same NUMA/die/package and reduces interference between workloads
  in balloons when system is not fully loaded. The default is `false`:
  pack new balloons tightly into the same NUMAs/dies/packages. This
  helps keeping large portions of hardware idle and entering into deep
  power saving states.
- `PreferSpreadOnPhysicalCores` prefers allocating logical CPUs
  (possibly hyperthreads) for a balloon from separate physical CPU
  cores. This prevents workloads in the balloon from interfering with
  themselves as they do not compete on the resources of the same CPU
  cores. On the other hand, it allows more interference between
  workloads in different balloons. The default is `false`: balloons
  are packed tightly to a minimum number of physical CPU cores. The
  value set here is the default for all balloon types, but it can be
  overridden with the balloon type specific setting with the same
  name.
- `BalloonTypes` is a list of balloon type definitions. Each type can
  be configured with the following parameters:
  - `Name` of the balloon type. This is used in pod annotations to
    assign containers to balloons of this type.
  - `Namespaces` is a list of namespaces (wildcards allowed) whose
    pods should be assigned to this balloon type, unless overridden by
    pod annotations.
  - `MinBalloons` is the minimum number of balloons of this type that
    is always present, even if the balloons would not have any
    containers. The default is 0: if a balloon has no containers, it
    can be destroyed.
  - `MaxBalloons` is the maximum number of balloons of this type that
    is allowed to co-exist. The default is 0: creating new balloons is
    not limited by the number of existing balloons.
  - `MaxCPUs` specifies the maximum number of CPUs in any balloon of
	this type. Balloons will not be inflated larger than this. 0 means
	unlimited.
  - `MinCPUs` specifies the minimum number of CPUs in any balloon of
    this type. When a balloon is created or deflated, it will always
    have at least this many CPUs, even if containers in the balloon
    request less.
  - `CpuClass` specifies the name of the CPU class according to which
    CPUs of balloons are configured.
  - `PreferSpreadingPods`: if `true`, containers of the same pod
    should be spread to different balloons of this type. The default
    is `false`: prefer placing containers of the same pod to the same
    balloon(s).
  - `PreferPerNamespaceBalloon`: if `true`, containers in the same
	namespace will be placed in the same balloon(s). On the other
	hand, containers in different namespaces are preferrably placed in
	different balloons. The default is `false`: namespace has no
	effect on choosing the balloon of this type.
  - `PreferNewBalloons`: if `true`, prefer creating new balloons over
    placing containers to existing balloons. This results in
    preferring exclusive CPUs, as long as there are enough free
    CPUs. The default is `false`: prefer filling and inflating
    existing balloons over creating new ones.
  - `ShareIdleCPUsInSame`: Whenever the number of or sizes of balloons
    change, idle CPUs (that do not belong to any balloon) are reshared
    as extra CPUs to workloads in balloons with this option. The value
    sets locality of allowed extra CPUs that will be common to these
    workloads.
    - `system`: workloads are allowed to use idle CPUs available
      anywhere in the system.
    - `package`: ...allowed to use idle CPUs in the same package(s)
    (sockets) as the balloon.
    - `die`: ...in the same die(s) as the balloon.
    - `numa`: ...in the same numa node(s) as the balloon.
    - `core`: ...allowed to use idle CPU threads in the same cores with
      the balloon.
  - `PreferSpreadOnPhysicalCores` overrides the policy level option
    with the same name in the scope of this balloon type.
  - `AllocatorPriority` (0: High, 1: Normal, 2: Low, 3: None). CPU
    allocator parameter, used when creating new or resizing existing
    balloons. If there are balloon types with pre-created balloons
    (`MinBalloons` > 0), balloons of the type with the highest
    `AllocatorPriority` are created first.

Related configuration parameters:
- `policy.ReservedResources.CPU` specifies the (number of) CPUs in the
  special `reserved` balloon. By default all containers in the
  `kube-system` namespace are assigned to the reserved balloon.
- `cpu.classes` defines CPU classes and their parameters (such as
  `minFreq`, `maxFreq`, `uncoreMinFreq` and `uncoreMaxFreq`).

### Example

Example configuration that runs all pods in balloons of 1-4 CPUs.
```yaml
policy:
  Active: balloons
  ReservedResources:
    CPU: 1
  balloons:
    PinCPU: true
    PinMemory: true
    IdleCPUClass: lowpower
    BalloonTypes:
      - Name: "quad"
        MinCpus: 1
        MaxCPUs: 4
        CPUClass: dynamic
        Namespaces:
          - "*"
cpu:
  classes:
    lowpower:
      minFreq: 800
      maxFreq: 800
    dynamic:
      minFreq: 800
      maxFreq: 3600
    turbo:
      minFreq: 3000
      maxFreq: 3600
      uncoreMinFreq: 2000
      uncoreMaxFreq: 2400
```

See the [sample configmap](/sample-configs/balloons-policy.cfg) for a
complete example.

## Assigning a Container to a Balloon

The balloon type of a container can be defined in pod annotations. In
the example below, the first annotation sets the balloon type (`BT`)
of a single container (`CONTAINER_NAME`). The last two annotations set
the default balloon type for all containers in the pod.

```yaml
balloon.balloons.cri-resource-manager.intel.com/container.CONTAINER_NAME: BT
balloon.balloons.cri-resource-manager.intel.com/pod: BT
balloon.balloons.cri-resource-manager.intel.com: BT
```

If a pod has no annotations, its namespace is matched to the
`Namespaces` of balloon types. The first matching balloon type is
used.

If the namespace does not match, the container is assigned to the
special `default` balloon, that means reserved CPUs unless `MinCPUs`
or `MaxCPUs` of the `default` balloon type are explicitely defined in
the `BalloonTypes` configuration.

## Metrics and Debugging

In order to enable more verbose logging and metrics exporting from the
balloons policy, enable instrumentation and policy debugging from the
CRI-RM global config:

```yaml
instrumentation:
  # The balloons policy exports containers running in each balloon,
  # and cpusets of balloons. Accessible in command line:
  # curl --silent http://localhost:8891/metrics
  HTTPEndpoint: :8891
  PrometheusExport: true
logger:
  Debug: policy
```
