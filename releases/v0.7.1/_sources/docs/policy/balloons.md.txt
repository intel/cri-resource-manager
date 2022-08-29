# Balloons Policy

## Overview

The balloons policy implements workload placement into "balloons" that
are pools of exclusive CPUs. A balloon can be inflated and deflated,
that is CPUs added and removed, based on the CPU resource requests of
the workloads in the balloon. The policy supports both static and
dynamically created and popped balloons. The balloons policy enables
configuring balloon-specific CPU classes.

## Deployment

### Install cri-resmgr

Deploy cri-resmgr on each node as you would for any other policy. See
[installation](../installation.md) for more details.

## Configuration

The balloons policy is configured using the yaml-based configuration
system of CRI-RM. See [setup and
usage](../setup.md#setting-up-cri-resource-manager) for more details
on managing the configuration.

Example configuration that runs all pods in balloons of 1-4 CPUs.
```yaml
policy:
  Active: balloons
  ReservedResources:
    CPU: 1
  balloons:
    BalloonTypes:
      - Name: "quad"
        MinCpus: 1
        MaxCPUs: 4
        Namespaces:
          - "*"
```

See the [sample configmap](/sample-configs/balloons-policy.cfg) for a
complete example.

### Debugging

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
