# Static-Pools (STP) Policy

## Overview

The `static-pools` (STP) builtin policy was inspired by [CMK (CPU Manager for
Kubernetes)][cmk]. It is an example policy demonstrating capabilities of
`cri-resource-manager` and not considered as production ready.

Basically, the STP policy aims to replicate the functionality of the `cmk
isolate` command of CMK. It also has compatibility features to function as
a drop-in replacement in order to allow easier testing and prototyping.

Features:

- arbitrary number of configurable CPU list pools
- dynamic configuration updates via the [node agent](../node-agent.md)

Please see the documentation of
[CMK][cmk] for a more detailed
description of the terminology and functionality.

CMK compatibility features:

- supports the same environment variables as the original CMK, except for:
  - `CMK_LOCK_TIMEOUT` and `CMK_PROC_FS`: configuration variables that are not
    applicable in cri-resmgr context
  - `CMK_LOG_LEVEL`: not implemented, yet
  - `CMK_NUM_CORES`: not needed in cri-resmgr as we take this value directly
    from the container resource request
- supports the existing configuration directory format of CMK for retrieving
  the pool configuration
- parses the container command/args in an attempt to retrieve command line
  options of `cmk isolate`
- supports generating CMK-specific node label and taint (off by default)

## Deployment

### Install cri-resmgr

Deploy cri-resmgr on each node as you would for any other policy. See
[installation](../installation.md) for more details.

### Deploy Node Agent

The CRI-RM node agent is required in order to communicate with the Kubernetes
control plane. In particular, the STP policy needs this capability for
updating the extended resource (that represents exclusive cores) as well as
managing legacy CMK node annotation and taint. In addition, the node agent
enables dynamic configuration updates.

See [node agent](../node-agent.md) for detailed instructions for set-up and
usage.

### Deploy Admission Webhook

You need to run and enable the cri-resmgr mutating admission webhook which
creates pod annotations consumed by CRI-RM. This is required so that the STP
policy is able to inspect the extended resources (in this case, exclusive CPU
cores) requested by containers.

See the [webhook](../webhook.md) for instructions how to set it up.

## Configuration

The policy is configured using the yaml-based configuration system of CRI-RM.
See [setup and usage](../setup.md#setting-up-cri-resource-manager) for more
details on managing the configuration.

At minimum, you need to specify the active policy in the configuration.
Policy-specific options control the pool configuration and legacy node label
and taint.

```yaml
policy:
  Active: static-pools
  static-pools:
    # Set to true to create CMK node label
    #LabelNode: false
    # Set to true to create CMK node taint
    #TaintNode: false
  ...
```

See the [sample configmap](/sample-configs/cri-resmgr-configmap.example.yaml)
for a complete example containing all available configuration options.

If dynamic configuration via the [node agent](../node-agent.md) is in use the
pools configuration may be altered at runtime.

**NOTE**: CMK legacy node labels and taints are not dynamically configurable in
CRI-RM v0.4.x.

**NOTE**: the active policy (`policy.Active`) cannot be changed at runtime. In
order to change the active policy cri-resmgr needs to be restarted.

### Pools Configuration

There are three possible sources of the pools configuration, in decreasing
priority order:

1. CRI-RM global config
1. stand-alone static-pools config file
1. CMK directory tree

The configuration is fully evaluated whenever a re-configuration event is
received (e.g. from the node agent). Thus, a valid pools config appearing in
the CRI-RM global config will take precedence over a directory tree based
config that was previously active. Similarly, removing pools config from the
CRI-RM global config will make a local config (file or directory tree)
effective.

**NOTE:** cri-resmgr does not have any utility for generating a pool
configuration. Thus, you need to either manually write one by yourself, or, run
the `cmk init` command (of the original CMK) in order to create a legacy
configuration directory structure.

#### Global Config

Configuration from the global CRI-RM config takes the highest preference, if
specified (under `policy.static-pools.pools`). A referential example:

```yaml
policy:
  static-pools:
    pools:
      exclusive:
        exclusive: true
        cpuLists:
        ...
      shared:
        cpuLists:
        ...
      infra:
        cpuLists:
        ...

```

#### Stand-alone YAML File

Path to a stand-alone configuration file can be specified with
`-static-pools-conf-file` (empty by default):

```bash
cri-resmgr -static-pools-conf-file "/path/to/conf.yaml"
```

Format of the configuration file is similar to the pools config used in the
global CRI-RM config. You can also see the
[example config file](/sample-configs/static-pools-policy.conf.example)
for a starting point.

#### CMK Directory Tree

The STP policy also supports configuration directory format of the original
CMK. It reads the configuration from a location specified with
`-static-pools-conf-dir` field (`/etc/cmk` by default):

```bash
cri-resmgr -static-pools-conf-dir /etc/cmk
```

### Command Line Flags

Cri-resmgr has some command line flags specific to the STP policy:

```text
  -static-pools-conf-dir string
        STP pool configuration directory (default "/etc/cmk")
  -static-pools-conf-file string
        STP pool configuration file
  -static-pools-create-cmk-node-label
        Create CMK-related node label for backwards compatibility
  -static-pools-create-cmk-node-taint
        Create CMK-related node taint for backwards compatibility
```


### Debugging

In order to enable more verbose logging for the STP policy specify
`-logger-debug static-pools` on the command line or enable debug from the CRI-RM global config:

```yaml
logger:
  Debug: static-pools

```

## Running Workloads

The preferred way to specify the pod configuration is through environment
variables. However, exclusive cores must be reserved by making a request of the
`cmk.intel.com/exclusive-cores` extended resource. Naming of the extended
resource has `cmk` prefix in order to provide backwards compatibility with the
original CMK.

### Pod Configuration Using Env Variables

The following environment variables are recognized:

| Name            | Description                                                |
| --------------- | ---------------------------------------------------------- |
| STP_NO_AFFINITY | Do not set cpu affinity. The workload is responsible for reading the `CMK_CPUS_ASSIGNED` environment variable and setting the affinity itself.
| STP_POOL        | Name of the pool to run in
| STP_SOCKET_ID   | Socket where cores should be allocated. Set to -1 to accept any socket.

An example Pod spec for running a workload in the `exclusive` pool with one
core reserved from socket id 0:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: stp-test
spec:
  containers:
  - name: stp-test
    image: busybox
    env:
      - name: STP_POOL
        value: "exclusive"
      - name: STP_SOCKET_ID
        value: "0"
    command:
      - "sh"
      - "-c"
      - "while :; do echo ASSIGNED: $CMK_CPUS_ASSIGNED; sleep 1; done"
    resources:
      requests:
        cmk.intel.com/exclusive-cores: "1"
      limits:
        cmk.intel.com/exclusive-cores: "1"
```

### Backwards Compatibility for `cmk isolate`

The STP policy parses the container command/args in an attempt to
retrieve the Pod configuration (from `cmk isolate` options). This is to provide
backwards compatibility with existing CMK workload specs. It manipulates the
container command and args so that `cmk isolate` and all it's arguments are
removed.

In the example below STP policy will run `sh -c "sleep 10000"` in the `infra`
pool.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cmk-test
spec:
  containers:
  - name: cmk-test
    image: busybox
    command:
      - "sh"
      - "-c"
    args:
      - "/opt/bin/cmk isolate --conf-dir=/etc/cmk --pool=infra sleep 10000"
```

<!-- Links -->
[cmk]: https://github.com/intel/CPU-Manager-for-Kubernetes
