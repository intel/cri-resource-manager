#!/usr/bin/env python3

"""numactlH2numajson - convert numactl -H output to numajson

Example:
  numactl -H | numactlH2numajson
"""

import json
import math
import re
import sys

QEMU_DEFAULT_DIST_OTHER = 20
QEMU_DEFAULT_DIST_SELF = 10

def error(msg, exit_status=1):
    sys.stderr.write("numactlH2numajson: %s\n" % (msg,))
    if not exit_status is None:
        sys.exit(1)

def round_size(size, size_unit, non_zero_numbers=3):
    if size_unit == "kB":
        size_mb = size / 1024
    elif size_unit == "MB":
        size_mb = size
    elif size_unit == "GB":
        size_mb = size * 1024
    elif size_unit == "TB":
        size_mb = size * 1024 * 1024
    else:
        raise Exception("unsupported size unit: %r" % (size_unit,))
    if size_mb == 0:
        return "0G"
    size_mul = 10**int(math.log10(size_mb))
    rounded = round(size_mb * 10**(non_zero_numbers-1) / size_mul) * size_mul / (10**(non_zero_numbers-1))
    if size_mul < 1000:
        return "%.0fM" % (rounded,)
    else:
        return "%.0fG" % (rounded/1000)

def add_dists_to_numalist(numalist, dists):
    """Add/replace distance information in numalist with node distances in dists.
       dists[i][j] = distance from node i to node j.
       dists can be a matrix or a dict: {sourcenode: {destnode: dist}}"""
    dist_matrix = []
    node = -1
    node_group = {} # {node: group_index_in_numalist}
    group_nodes = {} # {group_index_in_numalist: set_of_nodes}
    for groupindex, numaspec in enumerate(numalist):
        group_nodes[groupindex] = set()
        nodecount = int(numaspec.get("nodes", 1))
        for _ in range(nodecount):
            node += 1
            group_nodes[groupindex].add(node)
            node_group[node] = groupindex
    lastnode = node
    if isinstance(dists, list):
        # dists is a dist matrix.
        dist_matrix = dists
    else:
        # dists is a dict. create dist_matrix from it.
        for sourcenode in range(lastnode + 1):
            dist_matrix.append([])
            for destnode in range(lastnode + 1):
                if sourcenode in dists and destnode in dists[sourcenode]:
                    d = dists[sourcenode][destnode]
                elif sourcenode != destnode:
                    d = QEMU_DEFAULT_DIST_OTHER
                else:
                    d = QEMU_DEFAULT_DIST_SELF
                dist_matrix[-1].append(d)
    dist_freq = {} # {distance: number-of-appearances}
    try:
        for sourcenode in range(lastnode + 1):
            for destnode in range(lastnode + 1):
                if sourcenode != destnode:
                    d = dist_matrix[sourcenode][destnode]
                    dist_freq[d] = dist_freq.get(d, 0) + 1
    except IndexError:
        raise ValueError("invalid dists matrix dimensions, %sx%s expected" % (lastnode + 1, lastnode + 1))
    # Read the most common distance from the matrix, ignore distance-to-self.
    if len(dist_freq) > 0:
        default_dist = max([(v, k) for k, v in dist_freq.items()])[1]
    else:
        default_dist = QEMU_DEFAULT_DIST_SELF # don't care: there's only one node
    # Try filling symmetric distances with the default dist.
    # There may be asymmetry or node grouping that making this impossible.
    # In those cases sym_dist_errors > 0.
    sym_dist_errors = 0
    group_node_dist = {} # {group_index: {othernode: dist}}
    for sourcenode in range(lastnode + 1):
        sourcegroup = node_group[sourcenode]
        if not sourcegroup in group_node_dist:
            group_node_dist[sourcegroup] = {}
        for destnode in range(lastnode + 1):
            destgroup = node_group[destnode]
            if sourcenode == destnode:
                continue
            elif dist_matrix[sourcenode][destnode] == default_dist:
                continue
            elif dist_matrix[sourcenode][destnode] != dist_matrix[destnode][sourcenode]:
                # There is asymmetry.
                sym_dist_errors += 1
                continue
            for othernode in [n for n in group_nodes[sourcegroup] if n != sourcenode and n != destnode]:
                if (dist_matrix[othernode][destnode] != dist_matrix[sourcenode][destnode] or
                    dist_matrix[othernode][destnode] != dist_matrix[destnode][sourcenode]):
                    # Different nodes in the same group have different distances.
                    sym_dist_errors += 1
            group_node_dist[sourcegroup][destnode] = dist_matrix[sourcenode][destnode]
    # Clear existing distance definitions from numalist.
    for numaspec in numalist:
        if "dist" in numaspec:
            del numaspec["dist"]
        if "dist-all" in numaspec:
            del numaspec["dist-all"]
        if "node-dist" in numaspec:
            del numaspec["node-dist"]
    # Now we are ready to add distance information.
    if sym_dist_errors == 0 and len(str(group_node_dist)) < len(str(dist_matrix)):
        # Add info using "dist" and "node-dist", that is symmetrical distances.
        # This time it is more compact representation than a matrix.
        for groupindex, numaspec in enumerate(numalist):
            if group_node_dist[groupindex] != {}:
                # if all nodes mentioned in node-dist are in earlier groups,
                # there is no need to inject this definition, because it has been
                # covered by distance symmetry.
                nodes_with_dists = set(group_node_dist[groupindex].keys())
                for earlier_group in range(groupindex):
                    nodes_with_dists -= group_nodes[earlier_group]
                # there are new distance definitions, include all
                if len(nodes_with_dists) > 0:
                    numaspec["node-dist"] = group_node_dist[groupindex]
        if default_dist != QEMU_DEFAULT_DIST_OTHER:
            numalist[0]["dist"] = default_dist
    elif len(numalist) > 1:
        # Add distances as a matrix.
        numalist[-1]["dist-all"] = dist_matrix
    else:
        # There is no need for distance information in the numalist,
        # as there is only one node.
        pass

def numactlH2numajson(input_line_iter):
    numalist = []
    dist_matrix = []
    re_node_cpus = re.compile('^node (?P<node>[0-9]+) cpus:( (?P<cpus>([0-9]+\s?)*))?')
    re_node_size = re.compile('^node (?P<node>[0-9]+) size:( (?P<size>[0-9]+) (?P<size_unit>[a-zA-Z]+))?')
    re_node_distances = re.compile('^\s*(?P<sourcenode>[0-9]+):(?P<dists>(\s*[0-9]+)*)')
    for line in input_line_iter:
        m = re_node_cpus.match(line)
        if m:
            m_dict = m.groupdict()
            node = int(m_dict["node"])
            if m_dict["cpus"] is None:
                cpus = []
            else:
                cpus = [int(cpu) for cpu in m.groupdict()["cpus"].strip().split()]
            continue
        m = re_node_size.match(line)
        if m:
            m_dict = m.groupdict()
            if int(m_dict["node"]) != node:
                raise Exception("expected node %s size, got %r" % (node, line))
            size_unit = m_dict["size_unit"]
            mem = round_size(int(m_dict["size"]), size_unit)
            if (len(numalist) == 0
                or numalist[-1]["cpu"] != len(cpus)
                or numalist[-1]["mem"] != mem):
                # found a node that is different from the previous
                numalist.append({"cpu": len(cpus),
                                      "mem": mem,
                                      "nodes": 1})
            else:
                # found a node that looks the same as the previous
                numalist[-1]["nodes"] += 1
            nodecount = node + 1
            continue
        m = re_node_distances.match(line)
        if m:
            m_dict = m.groupdict()
            dist_matrix.append([int(d) for d in m_dict['dists'].strip().split()])

    # filter out unnecessary "nodes": 1 from the list:
    for d in numalist:
        if d["nodes"] == 1:
            del d["nodes"]
    # parse distances
    add_dists_to_numalist(numalist, dist_matrix)
    return numalist

def self_test():
    input_output = {
        """available: 5 nodes (0-4)
node 0 cpus: 0
node 0 size: 1007 MB
node 0 free: 784 MB
node 1 cpus: 1
node 1 size: 1007 MB
node 1 free: 262 MB
node 2 cpus: 2 3
node 2 size: 1951 MB
node 2 free: 1081 MB
node 3 cpus: 4 5 6 7
node 3 size: 4030 MB
node 3 free: 693 MB
node 4 cpus:
node 4 size: 8039 MB
node 4 free: 8029 MB
node distances:
node   0   1   2   3   4
  0:  10  22  22  22  88
  1:  22  10  22  22  88
  2:  22  22  10  22  88
  3:  22  22  22  10  88
  4:  88  88  88  88  10
""": [{'cpu': 1, 'mem': '1G', 'nodes': 2, 'node-dist': {4: 88}, 'dist': 22}, {'cpu': 2, 'mem': '2G', 'node-dist': {4: 88}}, {'cpu': 4, 'mem': '4G', 'node-dist': {4: 88}}, {'cpu': 0, 'mem': '8G'}],
        """available: 2 nodes (0-1)
node 0 cpus: 0 1 2 3
node 0 size: 3966 MB
node 0 free: 1649 MB
node 1 cpus: 4 5 6 7
node 1 size: 4006 MB
node 1 free: 983 MB
node distances:
node   0   1
  0:  10  20
  1:  20  10
""": [{'cpu': 4, 'mem': '4G', 'nodes': 2}],
"""available: 4 nodes (0-3)
node 0 cpus: 0 1 2 3
node 0 size: 3966 MB
node 0 free: 1649 MB
node 1 cpus: 4 5 6 7
node 1 size: 4006 MB
node 1 free: 983 MB
node 1 cpus: 8 9 10 11
node 1 size: 4006 MB
node 1 free: 983 MB
node 1 cpus: 12 13 14 15
node 1 size: 4006 MB
node 1 free: 983 MB
node distances:
node   0   1   2   3
  0:  10  55  55  55
  1:  55  10  55  55
  2:  55  55  10  55
  3:  55  55  55  10
""": [{'cpu': 4, 'mem': '4G', 'nodes': 4, 'dist': 55}],
    """available: 1 nodes (0)
node 0 cpus: 0 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19
node 0 size: 128000 MB
node 0 free: 80000 MB
node distances:
node   0
  0:  10
""": [{'cpu': 20, 'mem': '128G'}],
        """available: 5 nodes (0-4)
node 0 cpus: 0
node 0 size: 4007 MB
node 0 free: 784 MB
node 1 cpus: 1
node 1 size: 1007 MB
node 1 free: 262 MB
node 2 cpus: 2 3
node 2 size: 1951 MB
node 2 free: 1081 MB
node 3 cpus: 4 5 6 7
node 3 size: 4030 MB
node 3 free: 693 MB
node 4 cpus:
node 4 size: 8039 MB
node 4 free: 8029 MB
node distances:
node   0   1   2   3   4
  0:  10  22  33  44  55
  1:  22  10  22  22  22
  2:  33  22  10  22  22
  3:  44  22  22  10  22
  4:  55  22  22  22  10
""": [{'cpu': 1, 'mem': '4G', 'node-dist': {2: 33, 3: 44, 4: 55}, 'dist': 22}, {'cpu': 1, 'mem': '1G'}, {'cpu': 2, 'mem': '2G'}, {'cpu': 4, 'mem': '4G'}, {'cpu': 0, 'mem': '8G'}]
    }

    for input_string in input_output.keys():
        observed = numactlH2numajson(input_string.splitlines())
        expected = input_output[input_string]
        if observed != expected:
            raise Exception("self-test: observed/expected mismatch on numanodes\n%s\n\nobserved: %r\nexpected: %r" % (input_string, observed, expected))
    add_dists_to_numalist([], [])
    return 0

if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "test":
        sys.exit(self_test())
    try:
        numalist = numactlH2numajson(sys.stdin)
    except Exception as e:
        raise
        error(str(e))
    print(json.dumps(numalist))
