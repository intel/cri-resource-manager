# E2E static-pools policy test

## Requirements

This test requires containerd v1.4 or later on the VM. Earlier
containerd versions fail to mount container images built on top of
Clear Linux base image. That includes mounting cri-resmgr-webhook.

`cri-resmgr-webhook` image must be present on the host (`make
images`). The latest image in `docker images cri-resmgr-webhook` list
will be installed and tested on the VM.
