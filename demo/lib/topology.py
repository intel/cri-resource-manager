#!/usr/bin/env python3

"""topology.py - topology utility

Usage: topology.py [options] command

Options:
  -t TOPOLOGY_DUMP    load topology_dump from TOPOLOGY_DUMP file instead of
                      the topology_dump environment variable or local host.
  -c CPUS_ALLOWED     load cpus_allowed from CPUS_ALLOWED file instead of
                      the cpus_allowed environment variable or local host.

Commands:
  help                print help
  cpus                view cpu topology from topology_dump.
  cpus_allowed [PROCESS...]
                      view how matching procesess are allowed to use CPUs
                      in the CPU topology tree. If cpus_allowed data is not
                      read from CPUS_ALLOWED file or the cpus_allowed
                      environment variable, "pgrep -f PROCESS" is used to
                      match processes.
  bash_topology_dump  print a Bash command that creates topology_dump.
  bash_cpus_allowed PROCESS [PROCESS...]
                      print a Bash command that creates cpus_allowed dump
                      that contains Cpus_allowed masks of processes
                      matching "pgrep -f PROCESS".

Examples:
  Print local host CPU topology
  $ topology.py cpus

  Print how processes with pod0..2 in their names are allowed to use CPUs
  $ topology.py cpus_allowed pod0 pod1 pod2

  Print remote host CPU topology
  $ topology_dump="$(ssh remotehost "$(topology.py bash_topology_dump)")" topology.py cpus

  Watch how pod0..2 are allowed to CPUS on remote host, read topology only once
  $ export topology_dump="$(ssh remotehost "$(topology.py bash_topology_dump)")"
  $ watch 'cpus_allowed=$(ssh remotehost "$(topology.py bash_cpus_allowed pod0 pod1 pod2)") topology.py cpus_allowed'
"""

import getopt
import os
import re
import subprocess
import sys

_bash_topology_dump = """for cpu in /sys/devices/system/cpu/cpu[0-9]*; do cpu_id=${cpu#/sys/devices/system/cpu/cpu}; echo "cpu p:$(< ${cpu}/topology/physical_package_id) d:$(< ${cpu}/topology/die_id) n:$(basename  ${cpu}/node* | sed 's:node::g') c:$(< ${cpu}/topology/core_id) t:$(< ${cpu}/topology/thread_siblings) cpu:${cpu_id}" ; done;  for node in /sys/devices/system/node/node[0-9]*; do node_id=${node#/sys/devices/system/node/node}; echo "node dist $node_id: $(< $node/distance)"; echo "node mem $node_id: $(awk '/MemTotal/{print $4/1024}' < $node/meminfo) MB"; done"""

_bash_cpus_allowed = r"""for process in '%s'; do for pid in $(pgrep -f $process); do [ "$pid" != "$$" ] && [ "$pid" != "$PPID" ] && echo "${process}-${pid} $(awk '/Cpus_allowed:/{print $2}' < /proc/$pid/status)"; done; done"""

def error(msg, exit_status=1):
    """Print error message and exit."""
    if not msg is None:
        sys.stderr.write('topology.py: %s\n' % (msg,))
    if not exit_status is None:
        sys.exit(exit_status)

def warning(msg):
    """Print warning."""
    sys.stderr.write('topology.py warning: %s\n' % (msg,))

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

def get_local_cpus_allowed_dump(processes):
    """Return cpus_allowed from local system."""
    return bash_output(_bash_cpus_allowed % ("' '".join(processes),))

def dump_to_topology(dump):
    """Parse topology_dump, return topology data structures."""
    # Output data structures:
    tree = {} # {"package0": {"die1": {"node1": ...}}}
    cpu_branch = {} # {cpu_id: (package_name, die_name, node_name, core_name, thread_name, cpu_name)}
    # Example input line to be parsed:
    # "cpu p:0 d:1 n:3 c:2 t:00003000 cpu:13"
    re_dump_line = re.compile('cpu p:(?P<package>[0-9]+) d:(?P<die>[0-9]+) n:(?P<node>[0-9]+) c:(?P<core>[0-9]+) t:(?P<thread_siblings>[0-9a-f,]+) cpu:(?P<cpu_id>[0-9]+)')
    numeric_lines = []
    for line in dump.splitlines():
        try:
            mdict = re_dump_line.match(line).groupdict()
        except:
            continue
        package = int(mdict["package"])
        die = int(mdict["die"])
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
        numeric_lines.append((package, die, node, core, thread, cpu_id))
    max_package_len = max(len(str(nl[0])) for nl in numeric_lines)
    max_die_len = max(len(str(nl[1])) for nl in numeric_lines)
    max_node_len = max(len(str(nl[2])) for nl in numeric_lines)
    max_core_len = max(len(str(nl[3])) for nl in numeric_lines)
    max_thread_len = max(len(str(nl[4])) for nl in numeric_lines)
    max_cpu_id_len = max(len(str(nl[5])) for nl in numeric_lines)
    for (package, die, node, core, thread, cpu_id) in numeric_lines:
        branch = ("package" + str(package).zfill(max_package_len),
                  "die" + str(die).zfill(max_die_len),
                  "node" + str(node).zfill(max_node_len),
                  "core" + str(core).zfill(max_core_len),
                  "thread" + str(thread).zfill(max_thread_len),
                  "cpu" + str(cpu_id).zfill(max_cpu_id_len))
        add_tree(tree, branch, {})
        cpu_branch[cpu_id] = branch
    return {"tree": tree, "cpu_branch": cpu_branch}

def dump_to_cpus_allowed(cpus_allowed_dump):
    """Parse cpus_allowed data, return cpus_allowed data structure."""
    # Output data structure:
    owner_mask = {} # {owner_string: bitmask_int}
    # Example input line to be parsed:
    # "pod2  040c0000,00000000"
    re_owner_mask = re.compile(r'(?P<owner>[^ ]+)\s+(?P<mask>[0-9a-f,]+)')
    for line in cpus_allowed_dump.splitlines():
        try:
            mdict = re_owner_mask.match(line).groupdict()
        except:
            warning("cannot parse cpus_allowed line %r" % (line,))
        owner_mask[mdict["owner"]] = eval("0x" + mdict["mask"].replace(",", ""))
    return owner_mask

def get_topology():
    """Return topology data structure."""
    # Priority: use file, environment variable or read from local system
    if opt_topology_dump:
        topology_dump = opt_topology_dump
    else:
        topology_dump = os.getenv("topology_dump")
    if not topology_dump:
        topology_dump = get_local_topology_dump()
    return dump_to_topology(topology_dump)

def get_cpus_allowed(processes):
    """Return cpus_allowed data structure."""
    # Priority: use file, environment variable or read from local system
    if opt_cpus_allowed_dump:
        cpus_allowed_dump = opt_cpus_allowed_dump
    else:
        cpus_allowed_dump = os.getenv("cpus_allowed")
    if not cpus_allowed_dump:
        cpus_allowed_dump = get_local_cpus_allowed_dump(processes)
    return dump_to_cpus_allowed(cpus_allowed_dump)

def report_cpus():
    """Print CPU topology tree."""
    topology = get_topology()
    print(str_tree(topology["tree"]))

def report_cpus_allowed(processes):
    """Print CPU topology tree with chosen processes as leaf nodes."""
    topology = get_topology()
    tree = topology["tree"]
    cpu_branch = topology["cpu_branch"]
    max_cpu = max(cpu_branch.keys())
    cpus_allowed = get_cpus_allowed(processes)
    # add found owners to tree as children of cpus
    for owner, mask in sorted(cpus_allowed.items()):
        for cpu in range(max_cpu + 1):
            if mask & (1 << cpu):
                add_tree(tree, cpu_branch[cpu], {owner: {}})
    print(str_tree(tree))

if __name__ == "__main__":
    opt_topology_dump = None
    opt_cpus_allowed_dump = None
    options, commands = getopt.gnu_getopt(
        sys.argv[1:], 'ht:c:',
        ['help', '--topology-dump-file=', '--cpus-allowed-file='])
    for opt, arg in options:
        if opt in ["-h", "--help"]:
            print(__doc__)
            error(None, exit_status=0)
        elif opt in ["-t", "--topology-file"]:
            try:
                opt_topology_dump = open(arg).read()
            except IOError as e:
                error("cannot read topology dump from file %r: %s" % (arg, e))
        elif opt in ["-c", "--cpus-allowed-file"]:
            try:
                opt_cpus_allowed_dump = open(arg).read()
            except IOError as e:
                error("cannot read cpus_allowed dump from file %r: %s" % (arg, e))
    if not commands:
        error("missing command, see --help")
    elif commands[0] == "help":
        print(__doc__)
        error(None, exit_status=0)
    elif commands[0] == "cpus":
        report_cpus()
    elif commands[0] == "cpus_allowed":
        report_cpus_allowed(commands[1:])
    elif commands[0] == "bash_topology_dump":
        print(_bash_topology_dump)
    elif commands[0] == "bash_cpus_allowed":
        print(_bash_cpus_allowed % ("' '".join(commands[1:]),))
    else:
        error('invalid command %r' % (commands[0],))
