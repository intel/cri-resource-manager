#!/usr/bin/env python3

"""topology.py - topology utility

Usage: topology.py [options] command

Options:
  -t TOPOLOGY_DUMP    load topology_dump from TOPOLOGY_DUMP file instead of
                      the "topology_dump" environment variable or local host.
  -r RES_ALLOWED      load res_allowed from RES_ALLOWED file instead of
                      the "res_allowed" environment variable or local host.
  -o OUTPUT_FORMAT    "json" or "text". The default is "text".

Commands:
  help                print help
  cpus                view CPU topology from topology_dump.
  cpus_allowed [PROCESS...]
                      view how matching PROCESSes are allowed to use CPUs.
                      (Uses RES_ALLOWED like res_allowed below.)
  res                 view CPU and memory topology from topology_dump.
  res_allowed [PROCESS...]
                      view how matching PROCESSes are allowed to use CPUs
                      and memory in CPU/mem topology tree.
                      If the RES_ALLOWED file or the res_allowed
                      environment variable are not defined, "pgrep -f PROCESS"
                      is used to match processes.
  bash_topology_dump  print a Bash command that creates topology_dump.
  bash_res_allowed PROCESS [PROCESS...]
                      print a Bash command that creates res_allowed
                      dump that contains Cpus_allowed and Mems_allowed
                      masks of processes matching "pgrep -f PROCESS".

Examples:
  Print local host CPU topology
  $ topology.py cpus

  Print how processes with pod0..2 in their names are allowed to use CPUs
  $ topology.py res_allowed pod0 pod1 pod2

  Print remote host CPU topology
  $ topology_dump="$(ssh remotehost "$(topology.py bash_topology_dump)")" topology.py cpus

  Watch how pod0..2 are allowed to CPUS on remote host, read topology only once
  $ export topology_dump="$(ssh remotehost "$(topology.py bash_topology_dump)")"
  $ watch 'res_allowed=$(ssh remotehost "$(topology.py bash_res_allowed pod0 pod1 pod2)") topology.py res_allowed'
"""

import getopt
import json
import os
import re
import subprocess
import sys

_bash_topology_dump = """for cpu in /sys/devices/system/cpu/cpu[0-9]*; do cpu_id=${cpu#/sys/devices/system/cpu/cpu}; echo "cpu p:$(< ${cpu}/topology/physical_package_id) d:$(< ${cpu}/topology/die_id) n:$(basename  ${cpu}/node* | sed 's:node::g') c:$(< ${cpu}/topology/core_id) t:$(< ${cpu}/topology/thread_siblings) cpu:${cpu_id}" ; done;  for node in /sys/devices/system/node/node[0-9]*; do node_id=${node#/sys/devices/system/node/node}; echo "dist n:$node_id d:$(< $node/distance)"; echo "mem n:$node_id s:$(awk '/MemTotal/{print $4/1024}' < $node/meminfo)"; done"""

_bash_res_allowed = r"""for process in '%s'; do for pid in $(pgrep -f "$process"); do name=$(cat /proc/$pid/cmdline | tr '\0 ' '\n' | grep -E "^$process" | head -n 1); [ -n "$name" ] && [ "$pid" != "$$" ] && [ "$pid" != "$PPID" ] && echo "${name}/${pid} $(awk '/Cpus_allowed:/{c=$2}/Mems_allowed:/{m=$2}END{print "c:"c" m:"m}' < /proc/$pid/status)"; done; done"""

def error(msg, exit_status=1):
    """Print error message and exit."""
    if not msg is None:
        sys.stderr.write('topology.py: %s\n' % (msg,))
    if not exit_status is None:
        sys.exit(exit_status)

def warning(msg):
    """Print warning."""
    sys.stderr.write('topology.py warning: %s\n' % (msg,))

def output_tree(tree):
    """Print tree to output in OUTPUT_FORMAT"""
    if opt_output_format == "json":
        sys.stdout.write(json.dumps(tree))
    else:
        sys.stdout.write(str_tree(tree) + "\n")
    sys.stdout.flush()

def add_tree(root, branch, value_dict):
    """Add key-value pairs in value_dict to given branch in the tree starting from root.

    If the branch does not exist in the tree, it will be created.

    Example:
      add_tree(tree, ("package0", "die1", "node3", "core7", "thread0", "cpu15"), {"GHz", 4.2})
    """
    node = root
    for b in branch:
        if b in node:
            node = node[b]
        else:
            node[b] = {}
            node = node[b]
    node.update(value_dict)

def _str_node(root, lines, branch):
    """Format node names in tree to lines ([[line1col1, line1col2], ...])."""
    for key in sorted(root.keys()):
        branch.append(key)
        if root[key]:
            _str_node(root[key], lines, branch)
        else:
            # Add those column texts to the new line which does not have the same value
            # as previous non-empty text in the same column.
            new_line = []
            new_col_txt_added = False
            for col, txt in enumerate(branch):
                if new_col_txt_added:
                    prev_col_txt = ""
                else:
                    for prev_line in lines[::-1]:
                        if len(prev_line) > col and prev_line[col] != "":
                            prev_col_txt = prev_line[col]
                            break
                    else:
                        prev_col_txt = ""
                if txt != prev_col_txt:
                    new_line.append(txt)
                    new_col_txt_added = True
                else:
                    new_line.append("")
            lines.append(new_line)
        branch.pop()

def str_tree(root):
    """Format tree to string."""
    lines = []
    _str_node(root, lines, [])
    col_max_len = {} # {column-index: max-string-length}
    max_col = -1
    for line in lines:
        for col, txt in enumerate(line):
            if col > max_col:
                max_col = col
            if len(txt) > col_max_len.get(col, -1):
                col_max_len[col] = len(txt)
    str_lines = []
    for line in lines:
        line_cols = len(line)
        new_str_fmt = ""
        for col, txt in enumerate(line):
            new_str_fmt += "%-" + str(col_max_len[col] + 1) + "s"
        str_lines.append(new_str_fmt % tuple(line))
    return "\n".join(str_lines)

def bash_output(cmd):
    """Return standard output of executing cmd in Bash."""
    p = subprocess.Popen(["bash", "-c", cmd], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    out, err = p.communicate()
    return out.decode("utf-8")

def get_local_topology_dump():
    """Return topology_dump from local system."""
    return bash_output(_bash_topology_dump)

def get_local_res_allowed_dump(processes):
    """Return res_allowed from local system."""
    return bash_output(_bash_res_allowed % ("' '".join(processes),))

def dump_to_topology(dump, show_mem=True):
    """Parse topology_dump, return topology data structures."""
    # Output data structures:
    tree = {} # {"package0": {"die1": {"node1": ...}}}
    cpu_branch = {} # {cpu_id: (package_name, die_name, node_name, core_name, thread_name, cpu_name)}
    node_branch = {} # {node_id: (package_name, die_name, node_name)}
    mem_branch = {} # {node_id: (package_name, ...)}
    # Example input line to be parsed:
    # cpu line:
    # "cpu p:0 d:1 n:3 c:2 t:00003000 cpu:13"
    # mem line:
    # "mem n:4: s:8063.83"
    re_cpu_line = re.compile('cpu p:(?P<package>[0-9]+) d:(?P<die>[0-9]*) n:(?P<node>[0-9]+) c:(?P<core>[0-9]+) t:(?P<thread_siblings>[0-9a-f,]+) cpu:(?P<cpu_id>[0-9]+)')
    re_mem_line = re.compile('mem n:(?P<node>[0-9]+) s:(?P<size>[0-9.]+)')
    re_dist_line = re.compile('dist n:(?P<node>[0-9]+) d:(?P<dist>([0-9 ]+))')
    numeric_cpu_lines = []
    numeric_mem_lines = []
    numeric_dist_lines = []
    for line in dump.splitlines():
        m = re_cpu_line.match(line)
        if m:
            mdict = m.groupdict()
            package = int(mdict["package"])
            try:
                die = int(mdict["die"])
            except ValueError:
                die = 0 # handle kernels that do not provide topology/die_id
            node = int(mdict["node"])
            core = int(mdict["core"])
            thread_siblings = eval("0x" + mdict["thread_siblings"].replace(",", ""))
            cpu_id = int(mdict["cpu_id"])
            # Calculate thread id.
            # Let the lowest CPU bit owner in thread_siblings be thread 0, next thread 1 and so on.
            thread = -1
            bit = 1 << cpu_id
            while bit:
                if thread_siblings & bit:
                    thread += 1
                bit >>= 1
            numeric_cpu_lines.append((package, die, node, core, thread, cpu_id))
            continue
        m = re_mem_line.match(line)
        if m:
            mdict = m.groupdict()
            numeric_mem_lines.append((int(mdict["node"]), float(mdict["size"])))
            continue
        m = re_dist_line.match(line)
        if m:
            mdict = m.groupdict()
            numeric_dist_lines.append((int(mdict["node"]),
                                      tuple([int(n) for n in mdict["dist"].strip().split()])))
    numeric_mem_lines.sort() # make sure memory sizes are from node 0, 1, ...
    numeric_dist_lines.sort()

    # Build tree on CPUs
    max_package_len = max(len(str(nl[0])) for nl in numeric_cpu_lines)
    max_die_len = max(len(str(nl[1])) for nl in numeric_cpu_lines)
    max_node_len = max(len(str(nl[2])) for nl in numeric_cpu_lines)
    max_core_len = max(len(str(nl[3])) for nl in numeric_cpu_lines)
    max_thread_len = max(len(str(nl[4])) for nl in numeric_cpu_lines)
    max_cpu_id_len = max(len(str(nl[5])) for nl in numeric_cpu_lines)
    for (package, die, node, core, thread, cpu_id) in numeric_cpu_lines:
        branch = ("package" + str(package).zfill(max_package_len),
                  "die" + str(die).zfill(max_die_len),
                  "node" + str(node).zfill(max_node_len),
                  "core" + str(core).zfill(max_core_len),
                  "thread" + str(thread).zfill(max_thread_len),
                  "cpu" + str(cpu_id).zfill(max_cpu_id_len))
        add_tree(tree, branch, {})
        cpu_branch[cpu_id] = branch
        node_branch[node] = branch[:3]
    if show_mem:
        # Add node memory information to the tree
        for node, distvec in numeric_dist_lines:
            mem_node_name = "node" + str(node).zfill(max_node_len)
            node_mem_size = str(int(round((numeric_mem_lines[node][1]/1024)))) + "G"
            dists = sorted(distvec)
            if node in node_branch:
                # This node has CPU(s) as it has been added to the tree already in CPU lines.
                # Add memory branch to the tree under the existing node branch.
                branch = node_branch[node] + (
                    "mem", mem_node_name, node_mem_size)
            elif (dists[0] == 10 # sane distance-to-self
                  and (len(dists) < 3 or dists[1] < dists[2])  # there is a node closer than others
                  and distvec.index(dists[1]) in node_branch): # that node is already in the tree
                # This means that the node has the same memory controller as this node.
                # Add memory branch from this node under the existing node.
                node_same_ctrl = distvec.index(dists[1])
                branch = node_branch[node_same_ctrl] + (
                    "mem", mem_node_name, node_mem_size)
                node_branch[node] = branch[:3]
            else:
                # Suitable memory controller not found, create completely separate branch.
                branch = ("packagex", "mem", "node" + str(node).zfill(max_node_len),
                    "mem", mem_node_name, node_mem_size)
                node_branch[node] = branch[:3]
            add_tree(tree, branch, {})
            mem_branch[node] = branch
    return {"tree": tree,
            "cpu_branch": cpu_branch,
            "node_branch": node_branch,
            "mem_branch": mem_branch}

def dump_to_res_allowed(res_allowed_dump):
    """Parse res_allowed data, return allowed cpu and mem bitmasks in a data structure."""
    # Output data structure:
    owner_mask = {} # {owner_string: {"cpu": bitmask_int, "mem": bitmask_int}}
    # Example input line to be parsed:
    # "pod2  c:040c0000,00000000 m:00000000,00000300"
    re_owner_mask = re.compile(r'(?P<owner>[^ ]+)\s+c:(?P<cpumask>[0-9a-f,]+)\s+m:(?P<memmask>[0-9a-f,]+)')
    for line in res_allowed_dump.splitlines():
        if not line:
            continue
        try:
            mdict = re_owner_mask.match(line).groupdict()
        except:
            warning("cannot parse res_allowed line %r" % (line,))
            continue
        owner_mask[mdict["owner"]] = {
            "cpu": eval("0x" + mdict["cpumask"].replace(",", "")),
            "mem": eval("0x" + mdict["memmask"].replace(",", ""))
        }
    return owner_mask

def get_topology(show_mem=True):
    """Return topology data structure."""
    # Priority: use file, environment variable or read from local system
    if opt_topology_dump:
        topology_dump = opt_topology_dump
    else:
        topology_dump = os.getenv("topology_dump", None)
    if topology_dump is None:
        topology_dump = get_local_topology_dump()
    return dump_to_topology(topology_dump, show_mem=show_mem)

def get_res_allowed(processes):
    """Return res_allowed data structure."""
    # Priority: use file, environment variable or read from local system
    if opt_res_allowed_dump:
        res_allowed_dump = opt_res_allowed_dump
    else:
        res_allowed_dump = os.getenv("res_allowed", None)
    if res_allowed_dump is None:
        res_allowed_dump = get_local_res_allowed_dump(processes)
    return dump_to_res_allowed(res_allowed_dump)

def report_res(show_mem=True):
    """Print topology tree."""
    topology = get_topology(show_mem=show_mem)
    output_tree(topology["tree"])

def report_res_allowed(processes, show_mem=True):
    """Print topology tree with allowed processes as leaf nodes."""
    topology = get_topology(show_mem=show_mem)
    tree = topology["tree"]
    cpu_branch = topology["cpu_branch"]
    mem_branch = topology["mem_branch"]
    node_branch = topology["node_branch"]
    max_cpu = max(cpu_branch.keys())
    max_node = max(node_branch.keys())
    res_allowed = get_res_allowed(processes)
    # add found owners to tree as children of cpus
    for owner, masks in sorted(res_allowed.items()):
        cpumask = masks["cpu"]
        memmask = masks["mem"]
        for cpu in range(max_cpu + 1):
            if cpumask & (1 << cpu):
                add_tree(tree, cpu_branch[cpu], {owner: {}})
        if show_mem:
            for node in range(max_node + 1):
                if memmask & (1 << node):
                    add_tree(tree, mem_branch[node], {owner: {}})
    output_tree(tree)

if __name__ == "__main__":
    opt_topology_dump = None
    opt_res_allowed_dump = None
    opt_output_format = "text"
    try:
        options, commands = getopt.gnu_getopt(
            sys.argv[1:], 'ht:r:o:',
            ['help', '--topology-dump-file=', '--res-allowed-file='])
    except getopt.GetoptError as e:
        error(str(e))
    for opt, arg in options:
        if opt in ["-h", "--help"]:
            print(__doc__)
            error(None, exit_status=0)
        elif opt in ["-t", "--topology-file"]:
            try:
                opt_topology_dump = open(arg).read()
            except IOError as e:
                error("cannot read topology dump from file %r: %s" % (arg, e))
        elif opt in ["-r", "--res-allowed-file"]:
            try:
                opt_res_allowed_dump = open(arg).read()
            except IOError as e:
                error("cannot read res_allowed dump from file %r: %s" % (arg, e))
        elif opt in ["-o"]:
            if arg in ["json", "text"]:
                opt_output_format = arg
            else:
                error("invalid output format %r")
    if not commands:
        error("missing command, see --help")
    elif commands[0] == "help":
        print(__doc__)
        error(None, exit_status=0)
    elif commands[0] == "cpus":
        report_res(show_mem=False)
    elif commands[0] == "cpus_allowed":
        report_res_allowed(commands[1:], show_mem=False)
    elif commands[0] == "res":
        report_res(show_mem=True)
    elif commands[0] == "res_allowed":
        report_res_allowed(commands[1:])
    elif commands[0] == "bash_topology_dump":
        print(_bash_topology_dump)
    elif commands[0] == "bash_res_allowed":
        print(_bash_res_allowed % ("' '".join(commands[1:]),))
    else:
        error('invalid command %r' % (commands[0],))
