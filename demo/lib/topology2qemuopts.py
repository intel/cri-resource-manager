#!/usr/bin/env python3

"""topology2qemuopts - convert NUMA node list from JSON to Qemu options

NUMA node group definitions:
"mem"                 mem (RAM) size on each NUMA node in this group.
                      The default is "0G".
"nvmem"               nvmem (non-volatile RAM) size on each NUMA node
                      in this group. The default is "0G".
"dimm"                "": the default, memory is there without pc-dimm defined.
                      "plugged": start with cold plugged pc-dimm.
                      "unplugged": start with free slot for hot plug.
                        Add the dimm in Qemu monitor at runtime:
                          device_add pc-dimm,id=dimmX,memdev=memX,node=X
                        or
                          device_add nvdimm,id=nvdimmX,memdev=nvmemX,node=X
"cores"               number of CPU cores on each NUMA node in this group.
                      The default is 0.
"threads"             number of threads on each CPU core.
                      The default is 2.
"nodes"               number of NUMA nodes on each die.
                      The default is 1.
"dies"                number of dies on each package.
                      The default is 1.
"packages"            number of packages.
                      The default is 1.

NUMA node distances are defined with following keys:
"dist-all": [[from0to0, from0to1, ...], [from1to0, from1to1, ...], ...]
                      distances from every node to all nodes.
                      The order is the same as in to numactl -H
                      "node distances:" output.
"node-dist": {"node": dist, ...}
                      symmetrical distances from nodes in this group to other
                      nodes.

Distances that apply to all NUMA groups if defined in any:
"dist-same-die": N    the default distance between NUMA nodes on the same die.
"dist-same-package": N the default distance between NUMA nodes on the same package.
"dist-other-package": N  the default distance between NUMA nodes in other packages.

Note that the distance from a node to itself is always 10. The default
distance to a node on the same die is 11, and to other nodes on the
same and different packages is 21.

Example: Each of the first two NUMA groups in the list contains two
NUMA nodes. Each node in the first group includes two CPU cores and 2G
RAM, while nodes in the second group two CPU cores and 1G RAM. The
only NUMA node defined in the third group has 8G of NVRAM, and no CPU.

Every NUMA group with CPU cores adds a package (a socket) to the
configuration, or many identical packages if "packages" > 1.  This
example creates a two-socket system, four CPU cores per package. Note
that CPU cores are divided symmetrically to packages, meaning that
every NUMA group with CPU cores should contain the same number of
cores.

$ ( cat << EOF
[
    {
        "mem": "2G",
        "cores": 2,
        "nodes": 2
    },
    {
        "mem": "1G",
        "cores": 2,
        "nodes": 2
    },
    {
        "nvmem": "8G",
        "node-dist": {"0": 88, "1": 88, "2": 88, "3": 88,
                      "4": 66, "5": 66, "7": 66, "8": 66}
    }
]
EOF
) | python3 topology2qemuopts.py
"""

import sys
import json

DEFAULT_DIST = 21
DEFAULT_DIST_SAME_PACKAGE = 21
DEFAULT_DIST_SAME_DIE = 11
DEFAULT_DIST_SAME_NODE = 10

def error(msg, exitstatus=1):
    sys.stderr.write("topology2qemuopts: %s\n" % (msg,))
    if exitstatus is not None:
        sys.exit(exitstatus)

def siadd(s1, s2):
    if s1.lower().endswith("g") and s2.lower().endswith("g"):
        return str(int(s1[:-1]) + int(s2[:-1])) + "G"
    raise ValueError('supports only sizes in gigabytes, example: 2G')

def sisub(s1, s2):
    if s1.lower().endswith("g") and s2.lower().endswith("g"):
        return str(int(s1[:-1]) - int(s2[:-1])) + "G"
    raise ValueError('supports only sizes in gigabytes, example: 2G')

def validate(numalist):
    if not isinstance(numalist, list):
        raise ValueError('expected list containing dicts, got %s' % (type(numalist,).__name__))
    valid_keys = set(("mem", "nvmem", "dimm",
                      "cores", "threads", "nodes", "dies", "packages",
                      "node-dist", "dist-all",
                      "dist-other-package", "dist-same-package", "dist-same-die"))
    int_range_keys = {'cores': ('>= 0', lambda v: v >= 0),
                      'threads': ('> 0', lambda v: v > 0),
                      'nodes': ('> 0', lambda v: v > 0),
                      'dies': ('> 0', lambda v: v > 0),
                      'packages': ('> 0', lambda v: v > 0)}
    for numalistindex, numaspec in enumerate(numalist):
        for key in numaspec:
            if not key in valid_keys:
                raise ValueError('invalid name %r in node %r' % (key, numaspec))
            if key in ["mem", "nvmem"]:
                val = numaspec.get(key)
                if val == "0":
                    continue
                errmsg = 'invalid %s in node %r, expected string like "2G"' % (key, numaspec)
                if not isinstance(val, str):
                    raise ValueError(errmsg)
                try:
                    siadd(val, "0G")
                except ValueError:
                    raise ValueError(errmsg)
            if key in int_range_keys:
                try:
                    val = int(numaspec[key])
                    if not int_range_keys[key][1](val):
                        raise Exception()
                except:
                    raise ValueError('invalid %s in node %r, expected integer %s' % (key, numaspec, int_range_keys[key][0]))
        if 'threads' in numaspec and int(numaspec.get('cores', 0)) == 0:
            raise ValueError('threads set to %s but "cores" is 0 in node %r' % (numaspec["threads"], numaspec))

def dists(numalist):
    dist_dict = {} # Return value: {sourcenode: {destnode: dist}}, fully defined for all nodes
    sourcenode = -1
    lastsocket = -1
    dist_same_die = DEFAULT_DIST_SAME_DIE
    dist_same_package = DEFAULT_DIST_SAME_PACKAGE
    dist_other_package = DEFAULT_DIST # numalist "dist", if defined
    node_package_die = {} # topology {node: (package, die)}
    dist_matrix = None # numalist "dist_matrix", if defined
    node_node_dist = {} # numalist {sourcenode: {destnode: dist}}, if defined for sourcenode
    lastnode_in_group = -1
    for groupindex, numaspec in enumerate(numalist):
        nodecount = int(numaspec.get("nodes", 1))
        corecount = int(numaspec.get("cores", 0))
        diecount = int(numaspec.get("dies", 1))
        packagecount = int(numaspec.get("packages", 1))
        first_node_in_group = sourcenode + 1
        for package in range(packagecount):
            if nodecount > 0:
                lastsocket += 1
            for die in range(diecount):
                for node in range(nodecount):
                    sourcenode += 1
                    dist_dict[sourcenode] = {}
                    node_package_die[sourcenode] = (lastsocket, die)
        lastnode_in_group = sourcenode + 1
        if "dist" in numaspec:
            dist = numaspec["dist"]
        if "dist-same-die" in numaspec:
            dist_same_die = numaspec["dist-same-die"]
        if "dist-same-package" in numaspec:
            dist_same_package = numaspec["dist-same-package"]
        if "dist-all" in numaspec:
            dist_matrix = numaspec["dist-all"]
        if "node-dist" in numaspec:
            for n in range(first_node_in_group, lastnode_in_group):
                node_node_dist[n] = {int(nodename): value for nodename, value in numaspec["node-dist"].items()}
    if lastnode_in_group < 0:
        raise ValueError('no NUMA nodes found')
    lastnode = lastnode_in_group - 1
    if dist_matrix is not None:
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
                    dist_dict[sourcenode][destnode] = DEFAULT_DIST_SAME_NODE
                elif sourcenode in node_node_dist and destnode in node_node_dist[sourcenode]:
                    # User specified explicit node-to-node distance
                    dist_dict[sourcenode][destnode] = node_node_dist[sourcenode][destnode]
                    dist_dict[destnode][sourcenode] = node_node_dist[sourcenode][destnode]
                elif not destnode in dist_dict[sourcenode]:
                    # Set distance based on topology
                    if node_package_die[sourcenode] == node_package_die[destnode]:
                        dist_dict[sourcenode][destnode] = dist_same_die
                    elif node_package_die[sourcenode][0] == node_package_die[destnode][0]:
                        dist_dict[sourcenode][destnode] = dist_same_package
                    else:
                        dist_dict[sourcenode][destnode] = dist_other_package
    return dist_dict

def qemuopts(numalist):
    machineparam = "-machine pc"
    numaparams = []
    objectparams = []
    deviceparams = []
    lastnode = -1
    lastcpu = -1
    lastdie = -1
    lastsocket = -1
    lastmem = -1
    lastnvmem = -1
    totalmem = "0G"
    totalnvmem = "0G"
    unpluggedmem = "0G"
    pluggedmem = "0G"
    memslots = 0
    groupnodes = {} # groupnodes[NUMALISTINDEX] = (NODEID, ...)
    validate(numalist)

    # Read cpu counts, and "mem" and "nvmem" sizes for all nodes.
    threadcount = -1
    for numalistindex, numaspec in enumerate(numalist):
        nodecount = int(numaspec.get("nodes", 1))
        groupnodes[numalistindex] = tuple(range(lastnode + 1, lastnode + 1 + nodecount))
        corecount = int(numaspec.get("cores", 0))
        if corecount > 0:
            if threadcount < 0:
                # threads per cpu, set only once based on the first cpu-ful numa node
                threadcount = int(numaspec.get("threads", 2))
                threads_set_node = numaspec
            else:
                # threadcount already set, only check that there is no mismatch
                if (numaspec.get("threads", None) is not None and
                    threadcount != int(numaspec.get("threads"))):
                    raise ValueError('all CPUs must have the same number of threads, '
                                     'but %r had %s threads (the default) which contradicts %r' %
                                     (threads_set_node, threadcount, numaspec))
        cpucount = int(numaspec.get("cores", 0)) * threadcount # logical cpus per numa node (cores * threads)
        diecount = int(numaspec.get("dies", 1))
        packagecount = int(numaspec.get("packages", 1))
        memsize = numaspec.get("mem", "0")
        memdimm = numaspec.get("dimm", "")
        if memsize != "0":
            memcount = 1
        else:
            memcount = 0
        nvmemsize = numaspec.get("nvmem", "0")
        if nvmemsize != "0":
            nvmemcount = 1
        else:
            nvmemcount = 0
        for package in range(packagecount):
            if nodecount > 0 and cpucount > 0:
                lastsocket += 1
            for die in range(diecount):
                if nodecount > 0 and cpucount > 0:
                    lastdie += 1
                for node in range(nodecount):
                    lastnode += 1
                    currentnumaparams = []
                    for mem in range(memcount):
                        lastmem += 1
                        if memdimm == "":
                            objectparams.append("-object memory-backend-ram,size=%s,id=membuiltin_%s_node_%s" % (memsize, lastmem, lastnode))
                            currentnumaparams.append("-numa node,nodeid=%s,memdev=membuiltin_%s_node_%s" % (lastnode, lastmem, lastnode))
                        elif memdimm == "plugged":
                            objectparams.append("-object memory-backend-ram,size=%s,id=memdimm_%s_node_%s" % (memsize, lastmem, lastnode))
                            currentnumaparams.append("-numa node,nodeid=%s" % (lastnode,))
                            deviceparams.append("-device pc-dimm,node=%s,id=dimm%s,memdev=memdimm_%s_node_%s" % (lastnode, lastmem, lastmem, lastnode))
                            pluggedmem = siadd(pluggedmem, memsize)
                            memslots += 1
                        elif memdimm == "unplugged":
                            objectparams.append("-object memory-backend-ram,size=%s,id=memdimm_%s_node_%s" % (memsize, lastmem, lastnode))
                            currentnumaparams.append("-numa node,nodeid=%s" % (lastnode,))
                            unpluggedmem = siadd(unpluggedmem, memsize)
                            memslots += 1
                        else:
                            raise ValueError("unsupported dimm %r, expected 'plugged' or 'unplugged'" % (memdimm,))
                        totalmem = siadd(totalmem, memsize)
                    for nvmem in range(nvmemcount):
                        lastnvmem += 1
                        lastmem += 1
                        if lastnvmem == 0:
                            machineparam += ",nvdimm=on"
                        # Don't use file-backed nvdimms because the file would
                        # need to be accessible from the govm VM
                        # container. Everything is ram-backed on host for now.
                        if memdimm == "":
                            objectparams.append("-object memory-backend-ram,size=%s,id=memnvbuiltin_%s_node_%s" % (nvmemsize, lastmem, lastnode))
                            currentnumaparams.append("-numa node,nodeid=%s,memdev=memnvbuiltin_%s_node_%s" % (lastnode, lastmem, lastnode))
                        elif memdimm == "plugged":
                            objectparams.append("-object memory-backend-ram,size=%s,id=memnvdimm_%s_node_%s" % (nvmemsize, lastmem, lastnode))
                            currentnumaparams.append("-numa node,nodeid=%s" % (lastnode,))
                            deviceparams.append("-device nvdimm,node=%s,id=nvdimm%s,memdev=memnvdimm_%s_node_%s" % (lastnode, lastmem, lastmem, lastnode))
                            pluggedmem = siadd(pluggedmem, nvmemsize)
                            memslots += 1
                        elif memdimm == "unplugged":
                            objectparams.append("-object memory-backend-ram,size=%s,id=memnvdimm_%s_node_%s" % (nvmemsize, lastmem, lastnode))
                            currentnumaparams.append("-numa node,nodeid=%s" % (lastnode,))
                            unpluggedmem = siadd(unpluggedmem, nvmemsize)
                            memslots += 1
                        else:
                            raise ValueError("unsupported dimm %r, expected 'plugged' or 'unplugged'" % (memdimm,))
                        totalnvmem = siadd(totalnvmem, nvmemsize)
                    if cpucount > 0:
                        if not currentnumaparams:
                            currentnumaparams.append("-numa node,nodeid=%s" % (lastnode,))
                        currentnumaparams[-1] = currentnumaparams[-1] + (",cpus=%s-%s" % (lastcpu + 1, lastcpu + cpucount))
                        lastcpu += cpucount
                    numaparams.extend(currentnumaparams)
    node_node_dist = dists(numalist)
    for sourcenode in sorted(node_node_dist.keys()):
        for destnode in sorted(node_node_dist[sourcenode].keys()):
            if sourcenode == destnode:
                continue
            numaparams.append("-numa dist,src=%s,dst=%s,val=%s" % (
                sourcenode, destnode, node_node_dist[sourcenode][destnode]))
    if lastcpu == -1:
        raise ValueError('no CPUs found, make sure at least one NUMA node has "cores" > 0')
    if (lastdie + 1) // (lastsocket + 1) > 1:
        diesparam = ",dies=%s" % ((lastdie + 1) // (lastsocket + 1),)
    else:
        # Don't give dies parameter unless it is absolutely necessary
        # because it requires Qemu >= 5.0.
        diesparam = ""
    cpuparam = "-smp cpus=%s,threads=%s%s,sockets=%s" % (lastcpu + 1, threadcount, diesparam, lastsocket + 1)
    maxmem = siadd(totalmem, totalnvmem)
    startmem = sisub(sisub(maxmem, unpluggedmem), pluggedmem)
    memparam = "-m size=%s,slots=%s,maxmem=%s" % (startmem, memslots, maxmem)
    if startmem.startswith("0"):
        if pluggedmem.startswith("0"):
            raise ValueError('no memory in any NUMA node')
        raise ValueError("no initial memory in any NUMA node - cannot boot with hotpluggable memory")
    return (machineparam + " " +
            cpuparam + " " +
            memparam + " " +
            " ".join(numaparams) +
            " " +
            " ".join(deviceparams) +
            " " +
            " ".join(objectparams)
            )

def main(input_file):
    try:
        numalist = json.loads(input_file.read())
    except Exception as e:
        error("error reading JSON: %s" % (e,))
    try:
        print(qemuopts(numalist))
    except Exception as e:
        error("error converting JSON to Qemu opts: %s" % (e,))

if __name__ == "__main__":
    if len(sys.argv) > 1:
        if sys.argv[1] in ["-h", "--help"]:
            print(__doc__)
            sys.exit(0)
        else:
            input_file = open(sys.argv[1])
    else:
        input_file = sys.stdin
    main(input_file)
