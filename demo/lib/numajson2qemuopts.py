#!/usr/bin/env python3

"""numajson2qemuopts - convert NUMA node list from JSON to Qemu options

Example: Each of the two first groups contain two NUMA nodes. Nodes in
the first group include two CPUs and 2G RAM, nodes in the second group
single CPU and 1G RAM. The only NUMA node defined in the third group
has 8G of NVRAM, and no CPU.

$ ( cat << EOF
[
    {
        "cpu": 2,
        "mem": "2G",
        "nodes": 2
    },
    {
        "cpu": 1,
        "mem": "1G",
        "nodes": 2
    },
    {
        "nvmem": "8G",
        "dist": 20,
        "dist-group-0": 80,
        "dist-group-1": 80
    }
]
EOF
) | numajson2qemuopts

NUMA node group definitions:
"cpu"                 number of CPUs on every NUMA node in this group.
                      The default is 0.
"mem"                 mem (RAM) size on every NUMA node in this group.
                      The default is 0G.
"nvmem"               nvmem (non-volatile RAM) size on every NUMA node
                      in this group. The default is 0G.
"nodes"               number of NUMA nodes in this group. The default is 1.

NUMA node distances are defined with following keys:
"dist-group-X": N     symmetrical distance between nodes in this group and
                      nodes in group X. The first group in the list is group 0.
"dist-to-group-X": N  unidirectional distance from nodes in this group to
                      nodes in group X.
"dist-node-X": N      symmetrical distance between nodes in this group and
                      node X. The first node in the first group is node 0,
                      the second node in the first group is node 1, and so on.
"dist-to-node-X": N   unidirectional distance from nodes in this group to
                      node X.
"dist": N             the default distance on all node links within and between
                      all groups.

Note that the distance from a node to itself is always 10 (otherwise Qemu
would give an error).
"""

import sys
import json

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
    valid_keys = set(("cpu", "mem", "nvmem", "nodes", "dist"))
    valid_key_prefixes = set(("dist-group-", "dist-to-group-",
                              "dist-node-", "dist-to-node-"))
    for numalistindex, numaspec in enumerate(numalist):
        for key in numaspec:
            if key in valid_keys:
                continue
            for prefix in valid_key_prefixes:
                if key.startswith(prefix):
                    try:
                        v = int(key[len(prefix):])
                    except:
                        raise ValueError('integer expected in property %r after prefix %r' % (key, prefix))
                    break
            else:
                raise ValueError('invalid property name in numalist: %r' % (key,))

def qemuopts(numalist):
    machineparam = "-machine pc"
    numaparams = []
    objectparams = []
    lastnode = -1
    lastcpu = -1
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

    # Calculate distances in the order of precedence: nodes, groups and the default.
    found_dests = {src: set() for src in range(lastnode + 1)}
    # First, "dist-to-node" and "dist-node" override more general distances.
    sourcenode = -1
    for numalistindex, numaspec in enumerate(numalist):
        nodecount = int(numaspec.get("nodes", 1))
        for node in range(nodecount):
            sourcenode += 1
            if sourcenode not in found_dests[sourcenode]:
                # Mark all node-to-self-distances already defined, but
                # let Qemu use its default (10) for them instead of specifying
                # it explicitly (-numa dist,src=N,dst=N,val=10).
                # Qemu would give an error, if val != 10.
                found_dests[sourcenode].add(sourcenode)
            for destnode in range(lastnode + 1):
                destnodedist = numaspec.get("dist-to-node-%s" % (destnode,), None)
                symdestnodedist = numaspec.get("dist-node-%s" % (destnode,), None)
                if not destnodedist is None and destnode not in found_dests[sourcenode]:
                    numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (sourcenode, destnode, destnodedist))
                    found_dests[sourcenode].add(destnode)
                if not symdestnodedist is None:
                    if destnode not in found_dests[sourcenode]:
                        numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (sourcenode, destnode, symdestnodedist))
                        found_dests[sourcenode].add(destnode)
                    if sourcenode not in found_dests[destnode]:
                        numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (destnode, sourcenode, symdestnodedist))
                        found_dests[destnode].add(sourcenode)
    # Second, "dist-to-group" and "dist-group" override the default
    sourcenode = -1
    for numalistindex, numaspec in enumerate(numalist):
        nodecount = int(numaspec.get("nodes", 1))
        for node in range(nodecount):
            sourcenode += 1
            for destgroup in range(len(numalist)):
                groupdist = numaspec.get("dist-to-group-%s" % (destgroup,), None)
                symgroupdist = numaspec.get("dist-group-%s" % (destgroup,), None)
                if not groupdist is None:
                    for destnode in groupnodes[destgroup]:
                        if not destnode in found_dests[sourcenode]:
                            numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (sourcenode, destnode, groupdist))
                            found_dests[sourcenode].add(destnode)
                if not symgroupdist is None:
                    for destnode in groupnodes[destgroup]:
                        if not destnode in found_dests[sourcenode]:
                            numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (sourcenode, destnode, symgroupdist))
                            found_dests[sourcenode].add(destnode)
                        if not sourcenode in found_dests[destnode]:
                            numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (destnode, sourcenode, symgroupdist))
                            found_dests[destnode].add(sourcenode)
    # Finally, use the first found default distance for all other node links
    for numalistindex, numaspec in enumerate(numalist):
        defaultdist = numaspec.get("dist", None)
        if defaultdist is None:
            continue
        for sourcenode in range(lastnode + 1):
            for destnode in range(lastnode + 1):
                if not destnode in found_dests[sourcenode]:
                    numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (sourcenode, destnode, defaultdist))
                if not sourcenode in found_dests[destnode]:
                    numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (destnode, sourcenode, defaultdist))
    # Combine all parameters
    cpuparam = "-smp %s" % (lastcpu + 1,)
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
