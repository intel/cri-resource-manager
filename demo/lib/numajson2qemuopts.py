#!/usr/bin/env python3

"""numajson2qemuopts - convert NUMA node list from JSON to Qemu options

Example: Each of the first two NUMA groups in the list contains two
NUMA nodes. Each node in the first group includes two CPU cores and 2G
RAM, while nodes in the second group two CPU cores and 1G RAM. The
only NUMA node defined in the third group has 8G of NVRAM, and no CPU.

Every NUMA group with CPU cores adds a socket to the
configuration. That is, this example creates a two-socket system, four
CPU cores per socket. Note that CPU cores are divided symmetrically to
sockets, meaning that every NUMA group with CPU cores should contain
the same number of cores.

$ ( cat << EOF
[
    {
        "cpu": 2,
        "mem": "2G",
        "nodes": 2
    },
    {
        "cpu": 2,
        "mem": "1G",
        "nodes": 2
    },
    {
        "nvmem": "8G",
        "dist": 20,
        "node-dist": {"0": 88, "1": 88, "2": 88, "3": 88,
                      "4": 66, "5": 66, "7": 66, "8": 66}
    }
]
EOF
) | numajson2qemuopts

NUMA node group definitions:
"cpu"                 number of CPU cores on every NUMA node in this group.
                      The default is 0.
"mem"                 mem (RAM) size on every NUMA node in this group.
                      The default is 0G.
"nvmem"               nvmem (non-volatile RAM) size on every NUMA node
                      in this group. The default is 0G.
"nodes"               number of NUMA nodes in this group. The default is 1.

NUMA node distances are defined with following keys:
"dist-all": [[from0to0, from0to1, ...], [from1to0, from1to1, ...], ...]
                      distances from every node to all nodes.
                      The order is the same as in to numactl -H
                      "node distances:" output.
"node-dist": {"node": dist, ...}
                      symmetrical distances from nodes in this group to other
                      nodes.
"dist": N             the default distance, applies to all nodes in all node
                      groups.
Note that the distance from a node to itself is always 10 (otherwise Qemu
would give an error). The Qemu default distance between two nodes is 20.

"""

import sys
import json

QEMU_DEFAULT_DIST_OTHER = 20
QEMU_DEFAULT_DIST_SELF = 10

def error(msg, exitstatus=1):
    sys.stderr.write("numajson2qemuopts: %s\n" % (msg,))
    if not exitstatus is None:
        sys.exit(exitstatus)

def siadd(s1, s2):
    if s1.lower().endswith("g") and s2.lower().endswith("g"):
        return str(int(s1[:-1]) + int(s2[:-1])) + "G"
    raise ValueError('supports only sizes in gigabytes, example: 2G')

def validate(numalist):
    if not isinstance(numalist, list):
        raise ValueError('expected list containing dicts, got %s' % (type(numalist,).__name__))
    valid_keys = set(("cpu", "mem", "nvmem", "nodes", "dist", "node-dist", "dist-all"))
    for numalistindex, numaspec in enumerate(numalist):
        for key in numaspec:
            if not key in valid_keys:
                raise ValueError('invalid property name in numalist: %r' % (key,))

def dists(numalist):
    dist_dict = {} # Return value: {sourcenode: {destnode: dist}}, fully defined for all nodes
    sourcenode = -1
    dist = QEMU_DEFAULT_DIST_OTHER # numalist "dist", if defined
    dist_matrix = None # numalist "dist_matrix", if defined
    node_node_dist = {} # numalist {sourcenode: {destnode: dist}}, if defined for sourcenode
    for groupindex, numaspec in enumerate(numalist):
        nodecount = int(numaspec.get("nodes", 1))
        first_node_in_group = sourcenode + 1
        for node in range(nodecount):
            sourcenode += 1
            dist_dict[sourcenode] = {}
        lastnode_in_group = sourcenode + 1
        if "dist" in numaspec:
            dist = numaspec["dist"]
        if "dist-all" in numaspec:
            dist_matrix = numaspec["dist-all"]
        if "node-dist" in numaspec:
            for n in range(first_node_in_group, lastnode_in_group):
                node_node_dist[n] = {int(nodename): value for nodename, value in numaspec["node-dist"].items()}
    lastnode = lastnode_in_group - 1
    if not dist_matrix is None:
        # Fill the dist_dict directly from dist_matrix.
        # It must cover all distances.
        if len(dist_matrix) != lastnode + 1:
            raise ValueError("wrong dimensions in dist-all %s rows seen, %s expected" % (len(dist_matrix), lastnode))
        for sourcenode, row in enumerate(dist_matrix):
            if len(row) != lastnode + 1:
                raise ValueError("wrong dimensions in dist-all on row %s: %s distances seen, %s expected" % (sourcenode + 1, len(row), lastnode + 1))
            for destnode, source_dest_dist in enumerate(row):
                dist_dict[sourcenode][destnode] = source_dest_dist
    else:
        for sourcenode in range(lastnode + 1):
            for destnode in range(lastnode + 1):
                if sourcenode == destnode:
                    dist_dict[sourcenode][destnode] = QEMU_DEFAULT_DIST_SELF
                elif sourcenode in node_node_dist and destnode in node_node_dist[sourcenode]:
                    dist_dict[sourcenode][destnode] = node_node_dist[sourcenode][destnode]
                    dist_dict[destnode][sourcenode] = node_node_dist[sourcenode][destnode]
                elif not destnode in dist_dict[sourcenode]:
                    dist_dict[sourcenode][destnode] = dist
    return dist_dict

def qemuopts(numalist):
    machineparam = "-machine pc"
    numaparams = []
    objectparams = []
    lastnode = -1
    lastcpu = -1
    lastsocket = -1
    lastmem = -1
    lastnvmem = -1
    totalmem = "0G"
    totalnvmem = "0G"
    groupnodes = {} # groupnodes[NUMALISTINDEX] = (NODEID, ...)
    validate(numalist)

    # Read  "cpu" counts, and "mem" and "nvmem" sizes for all nodes.
    for numalistindex, numaspec in enumerate(numalist):
        nodecount = int(numaspec.get("nodes", 1))
        groupnodes[numalistindex] = tuple(range(lastnode + 1, lastnode + 1 + nodecount))
        cpucount = int(numaspec.get("cpu", 0))
        memsize = numaspec.get("mem", "0")
        if nodecount > 0 and cpucount > 0:
            lastsocket += 1
        if memsize != "0":
            memcount = 1
        else:
            memcount = 0
        nvmemsize = numaspec.get("nvmem", "0")
        if nvmemsize != "0":
            nvmemcount = 1
        else:
            nvmemcount = 0
        for node in range(nodecount):
            lastnode += 1
            for mem in range(memcount):
                lastmem += 1
                objectparams.append("-object memory-backend-ram,size=%s,id=mem%s" % (memsize, lastmem))
                numaparams.append("-numa node,nodeid=%s,memdev=mem%s" % (lastnode, lastmem))
                totalmem = siadd(totalmem, memsize)
            for nvmem in range(nvmemcount):
                lastnvmem += 1
                if lastnvmem == 0:
                    machineparam += ",nvdimm=on"
                # Don't use file-backed nvdimms because the file would
                # need to be accessible from the govm VM
                # container. Everything is ram-backed on host for now.
                objectparams.append("-object memory-backend-ram,size=%s,id=nvmem%s" % (nvmemsize, lastnvmem))
                # Currently nvdimm is not backed up by -device pair.
                numaparams.append("-numa node,nodeid=%s,memdev=nvmem%s" % (lastnode, lastnvmem))
                totalnvmem = siadd(totalnvmem, nvmemsize)
            if cpucount > 0:
                numaparams[-1] = numaparams[-1] + (",cpus=%s-%s" % (lastcpu + 1, lastcpu + cpucount))
                lastcpu += cpucount
    node_node_dist = dists(numalist)
    for sourcenode in sorted(node_node_dist.keys()):
        for destnode in sorted(node_node_dist[sourcenode].keys()):
            if sourcenode == destnode:
                continue
            numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (
                sourcenode, destnode, node_node_dist[sourcenode][destnode]))

    cpuparam = "-smp cpus=%s,sockets=%s" % (lastcpu + 1, lastsocket + 1)
    memparam = "-m %s" % (siadd(totalmem, totalnvmem),)
    return (machineparam + " " +
            cpuparam + " " +
            memparam + " " +
            " ".join(numaparams) + " " +
            " ".join(objectparams))

def main():
    try:
        numalist = json.loads(sys.stdin.read())
    except Exception as e:
        error("error reading JSON from stdin: %s" % (e,))
    try:
        print(qemuopts(numalist))
    except Exception as e:
        error("error converting JSON to Qemu opts: %s" % (e,))

if __name__ == "__main__":
    if len(sys.argv) > 1:
        print(__doc__)
        sys.exit(0)
    main()
