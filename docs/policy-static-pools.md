# Static-Pools (STP) Policy

## Overview

The `static-pools` (STP) builtin policy was inspired by [CMK (CPU Manager for
Kubernetes)](https://github.com/intel/CPU-Manager-for-Kubernetes). It is an
example policy demonstrating some capabilities of `cri-resource-manager` - not
production ready.

Basically, the STP policy tries to replicate the functionality of the `cmk
isolate` command CMK. The STP policy also has some compatibility features to
function as close as possible drop-in replacement in order to allow easier
testing and prototyping.

Features:
- arbitrary number of configurable CPU list pools
- dynamic configuration updates via `cri-resmgr-agent`

Please see the documentation of
[CMK](https://github.com/intel/CPU-Manager-for-Kubernetes) for a more detailed
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

### Configuration

The STP policy tries to use configuration received from the `cri-resmgr-agent`
which is, in turn, read from a Kubernetes ConfigMap.

As a fallback, the STP policy also supports configuration directory format of
the original CMK. It tries to read the configuration from a location specified
with `-static-pools-conf-dir` (`/etc/cmk` by default).
Alternatively, you can provide a configuration file (YAML) by using the
`-static-pools-conf-file` flag.
See the [example config](/sample-configs/static-pools-policy.conf.example) for a
starting point.  However, if `cri-resmgr` at a later time receives a valid
configuration from the `cri-resmr-agent` this will override the fallback
configuration read from the directory or file.

**NOTE:** cri-resmgr does not have any utility for automatically generating a
configuration. Thus, you need to either manually write one by yourself, or, run
the `cmk init` command (of the original CMK) in order to create a legacy
configuration directory structure.

### Install cri-resmgr

Deploy cri-resmgr on each node as you would for any other policy.

### Deploy Admission Webhook

You need to run and enable the cri-resmgr mutating admission webhook which is
making resource request annotations. This is required so that the STP policy is
able to inspect the extended resources (in this case, exclusive CPU cores)
requested by containers. See the [README](../README.md) for instructions.

## Running cri-resmgr

An example of running cri-resmgr with the STP policy enabled, creating legacy
CMK node label and taint:
```
cri-resmgr -policy static-pools -logger-debug stp -static-pools-create-cmk-node-label -static-pools-create-cmk-node-taint
```

### Command Line Flags

Cri-resmgr has some command line flags specific to the STP policy:
```
  -static-pools-conf-dir string
        STP pool configuration directory (default "/etc/cmk")
  -static-pools-conf-file string
        STP pool configuration file
  -static-pools-create-cmk-node-label
        Create CMK-related node label for backwards compatibility
  -static-pools-create-cmk-node-taint
        Create CMK-related node taint for backwards compatibility
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
```
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

### Legacy CMK Pod Configuration

The STP policy tries to parse the container command/args in an attempt to
retrieve the Pod configuration (from `cmk isolate` options). This is to provide
backwards compatibility with existing CMK workload specs.

```
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
