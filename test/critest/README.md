# CRI Resource Manager: Container Runtime Interface Validation

This test runs
[`critest`](https://github.com/kubernetes-sigs/cri-tools/blob/master/docs/validation.md)
from [cri-tools](https://github.com/kubernetes-sigs/cri-tools/) to
make sure that various `cri-resmgr` configurations do not break CRI
runtime conformance.

## Prerequisites

Install:
- `docker`
- `govm`

## Run the test

```
./run.sh test
```
