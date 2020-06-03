# CRI Resoure Manager: Block I/O Demo

This demo creates a virtual machine for a single-node Kubernetes
cluster where container runtime features are extended by `cri-resmgr`.

In this setup `cri-resmgr` is configured with block I/O parameters
that throttles I/O bandwith of a container that constantly scans
system file checksums.

## Prerequisites

Install:
- `docker`
- `govm`

## Run the demo

```
./run.sh play
```

The demo does not delete the virtual machine so that you can
experiment with it. You can login to the virtual machine:

```
$ govm ssh crirm-demo-blockio
```

## Clean up - and run the demo from scratch

In order to run the demo from scratch again, delete the virtual
machine:

```
$ govm delete crirm-demo-blockio
```
