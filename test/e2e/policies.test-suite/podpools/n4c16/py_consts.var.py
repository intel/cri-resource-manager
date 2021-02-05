# This file captures expected CPU allocator behavior when the podpools
# policy is started with the test default cri-resmgr configuration on
# n4c16 topology.

# cri-resmgr output on constructed pools.
expected_podpools_output = """
podpools policy pools:
- pool 0: reserved[0]{cpus:15, mems:3, pods:0/0, containers:0}
- pool 1: default[0]{cpus:5,12-14, mems:1,3, pods:0/0, containers:0}
- pool 2: singlecpu[0]{cpus:2, mems:0, pods:0/2, containers:0}
- pool 3: singlecpu[1]{cpus:3, mems:0, pods:0/2, containers:0}
- pool 4: singlecpu[2]{cpus:4, mems:1, pods:0/2, containers:0}
- pool 5: dualcpu[0]{cpus:6-7, mems:1, pods:0/3, containers:0}
- pool 6: dualcpu[1]{cpus:8-9, mems:2, pods:0/3, containers:0}
- pool 7: dualcpu[2]{cpus:10-11, mems:2, pods:0/3, containers:0}
"""

# 1. Parse expected_podpools_output into
#    expected.cpus.POOLNAME[INSTANCE] = {"cpuNN", ...}
# 2. Calculate memory nodes based on expected.cpus into
#    expected.mems.POOLNAME[INSTANCE] = {"nodeN", ...}
#    (do not read these from output in order to verify its correctness)
#
# As the result:
# expected.cpus.singlecpu == [{"cpu02"}, {"cpu03"}, {"cpu04"}]
# expected.mems.singlecpu == [{"node0"}, {"node0"}, {"node1"}]

import re

class expected:
    class cpus:
        pass
    class mems:
        pass

def _add_expected_pool(poolname, poolindex, cpuset):
    cpus = []
    for cpurange in cpuset.split(","):
        lower_upper = [int(n) for n in cpurange.split("-")]
        if len(lower_upper) == 1:
            cpus.append(lower_upper[0])
        else:
            cpus.extend([i for i in range(lower_upper[0], lower_upper[1]+1)])
    if not hasattr(expected.cpus, poolname):
        setattr(expected.cpus, poolname, [])
        setattr(expected.mems, poolname, [])
    getattr(expected.cpus, poolname).append(set('cpu%s' % (str(cpu).zfill(2),) for cpu in cpus))
    getattr(expected.mems, poolname).append(set("node%s" % (cpu//4,) for cpu in cpus))

for poolname, poolindex, cpuset in re.findall(r': ([a-z]+)\[([0-9]+)\]\{cpus:([0-9,-]+), ', expected_podpools_output):
    _add_expected_pool(poolname, poolindex, cpuset)
