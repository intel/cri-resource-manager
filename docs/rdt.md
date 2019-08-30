# RDT (Intel® Resource Director Technology)

## Overview

Intel® RDT provides capabilities for cache and memory allocation and
monitoring. In Linux system the functionality is exposed to the user space via
the [resctrl](https://www.kernel.org/doc/Documentation/x86/intel_rdt_ui.txt)
filesystem. Cache and memory allocation in RDT is handled by using resource
control groups. Resource allocation is specified on the group level and each
task (process/thread) is assigned to one group.

Out of the RDT technologies, CRI Resource Manager currently supports L3 cache
allocation and memory bandwidth allocation. Based on the configuration, it
creates a set of resource control groups which the policies can assign
containers to.

## Configuration

### Command Line Flags

| Flag      | Description                           |
| --------- | ------------------------------------- |
| `-no-rdt` | Disable RDT resource management

### Dynamic Configuration

CRI utilizes configuration received from
[`cri-resmgr-agent`](../README.md#cri-resource-manager-node-agent), under the
key `rdt` in the ConfigMap containing `cri-resmgr` configuration data. The
configuration specifies a set of RDT classes (or resource control groups) that
the policies assign containers to. The configuration can be dynamically updated
by editing the ConfigMap. The set of RDT classes can be freely specified, but,
one must ensure that classes required by the active policy are specified, and,
that the maxmimum number of classes (CLOSes) supported by the underlying system
is not exceeded.

CRI-RM has a built-in default configuration containing three classes
corresponding to the Pod QOS classes of Kubernetes. These are utilized by the
`static` policy.

See `rdt` in the [example ConfigMap spec](../sample-configs/cri-resmgr-configmap.example.yaml)
for an example configuration.
