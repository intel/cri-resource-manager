#!/bin/bash

RUN_SH="${0%/*}/run.sh"
PAIRWISE="${0%/*}/../../scripts/testing/pairwise"

"${PAIRWISE}" \
    distro={debian-sid,fedora-35,fedora-34,opensuse-tumbleweed} \
    k8scri={containerd,crio,cri-resmgr\|containerd,cri-resmgr\|crio} \
    k8scni={cilium,flannel,weavenet} | while read -r env_vars; do

    eval "export $env_vars"

    code='create besteffort'
    # shellcheck disable=SC2154
    # ...as it cannot know that pairwise+eval exports distro et. al.
    vm="config-$distro-${k8scri/|/-}-$k8scni"
    outdir="output-configs/output-$vm"
    export code vm outdir

    govm rm "$vm" >/dev/null 2>&1
    mkdir -p "$outdir"
    "$RUN_SH" test </dev/null >"$outdir/run.sh.output" 2>&1
    govm rm "$vm" >/dev/null 2>&1
done
