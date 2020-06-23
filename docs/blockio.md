# Block IO

## Overview

Block IO controller provides means to control
- block device IO scheduling priority (weight)
- throttling IO bandwith
- throttling number of IO operations.

CRI Resource Manager applies block IO contoller parameters to pods via
[cgroups block io contoller](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v1/blkio-controller.html).

## Configuration

See [sample blockio configuration](/sample-configs/blockio.cfg).

## Demo

See [Block IO demo](/demo/blockio/README.md)
