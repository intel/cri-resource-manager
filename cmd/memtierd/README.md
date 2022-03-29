# memtierd - daemon for moving memory between NUMA nodes

Memtierd is a userspace daemon that manages memory of chosen
processes. Memtierd supports reclaiming memory and moving memory
between NUMA nodes. Moving enables both promotion and demotion of
pages, that is, moving actively used pages to low-latency memory, and
idle pages away from low-latency memory to free it for better use.

Memtierd includes alternative memory trackers and policies. A tracker
counts accesses of memory pages, while a policy classifies the pages
based on observed accesses: is a page active, idle, or somewhere
between.

The granularity of memtierd trackers and memory classifications is
configurable, and often significantly larger than a single page. For
simplicity, this document talks about "pages", but most often this
means an address range that contains one or more pages.

## Build

```
make bin/memtierd
```

## Usage

Memtierd starts in an automatic mode with a configuration file, or in
a command mode with interactive prompt.

### Automatic mode

In the automatic mode memtierd configures a policy that starts
managing pages. Memtierd enters to the automatic mode when launched
with a configuration file that includes policy and tracker parameters:

```
memtierd -config FILE
```

See configuration samples below.

- [memtierd-age-idlepage-trackonly.yaml](../../sample-configs/memtierd-age-idlepage-trackonly.yaml)
  tracks processes in `/sys/fs/cgroup/track-me` but does not swap out or
  move memory. Useful for understanding memory access time
  demographics. Example:
  ```
  # (while :; do sleep 5; echo policy -dump accessed 0,5s,30s,10m,2h,24h,0; done) | memtierd -config memtierd-age-idlepage-trackonly.yaml
  ...
  memtierd> policy -dump accessed 0,5s,30s,10m,2h,24h,0
  table: time since last access
       pid lastacc>=[s] lastacc<[s]    pages   mem[M] pidmem[%]
   2857761        0.000       5.000    99438      388     35.37
   2857761        5.000      30.000        0        0      0.00
   2857761       30.000     600.000   181697      709     64.63
   2857761      600.000    7200.000        0        0      0.00
   2857761     7200.000   86400.000        0        0      0.00
   2857761    86400.000       0.000        0        0      0.00
  ```

- [memtierd-heat-damon.yaml](../../sample-configs/memtierd-heat-damon.yaml)
  configures the heat policy to use the damon tracker.

- [memtierd-heat-idlepage.yaml](../../sample-configs/memtierd-heat-idlepage.yaml)
  configures the heat policy to use the idlepage tracker.

- [memtierd-age-softdirty.yaml](../../sample-configs/memtierd-age-softdirty.yaml)
  configures the age policy to use the softdirty tracker.

Policies are described in the [Policies](#policies) section.

### Command mode

In the command mode memtierd reads user commands from the standard
input and prints results to the standard output. Memtierd enters to
the command mode when launched with `-prompt`. The command mode prints
help on available commands with `help`, and on parameters of a command
with `COMMAND -h`:

```
memtierd -prompt
memtierd> help
memtierd> pages -h
```

If a command includes a pipe (`|`), the right-hand-side of the first
pipe will be run in a shell, and the output of the left-hand-side of
the pipe will be piped to the shell command:

```
memtierd> stats | grep accessed
```

Example: Start moving all pages of the `meme` process to NUMA
node 1. After 10 seconds print statistics and quit:

```
( echo "pages -pid $(pidof meme)"; echo "mover -pages-to 1"; sleep 10; echo "stats"; echo "q" ) | memtierd -prompt
```

Example: Use idlepage tracker to track the memory of the `meme`
process. After 10 seconds print detected accesses and statistics:

```
( echo "tracker -create idlepage -start $(pidof meme)"; sleep 10; echo "tracker -counters"; echo "stats" ) | ./memtierd -prompt
```

> Tip: install `rlwrap` and run `sudo rlwrap memtierd --prompt` to
> enable convenient readline input with history.

## Test environment

This section describes how to use CRI Resource manager's e2e test
framework in creating a virtual machine (VM) for testing. The test
framework allows specifying hardware topology, including the number of
CPUs and amount of memory in each NUMA nodes, and choosing Linux
distribution to be installed on the VM.

The e2e test framework uses
[govm](https://github.com/govm-project/govm) that runs Qemu VMs in
docker containers.

### Install e2e test framework dependencies on host

1. docker
2. govm

   Example on building govm and govm/govm:latest Docker image on Ubuntu:
   ```
   sudo apt install -y docker.io git-core golang
   export GOPATH=$HOME/go
   export PATH=$PATH:$GOPATH/bin
   GO111MODULE=off go get -d github.com/govm-project/govm && cd $GOPATH/src/github.com/govm-project/govm && go mod tidy && go mod download && go install && cd .. && docker build govm -f govm/Dockerfile -t govm/govm:latest
   ```

### Create a VM with the topology of your interest

Example of a four-NUMA-node topology with:
- 2 NUMA nodes with 4 CPUs / 4G memory on each node
- 2 NUMA nodes with 0 CPUs / 2G memory on each node

```
topology='[{"cores": 2, "mem": "4G", "nodes": 2}, {"cores":0, "mem":"2G", "nodes": 2}]' distro=opensuse-tumbleweed vm=opensuse-4422 on_vm_online='interactive; exit' test/e2e/run.sh interactive
```

See supported Linux distributions and other options with
`test/e2e/run.sh help`.

You can get help on all available commands in the interactive prompt
and in the scripts:

```
test/e2e/run.sh help script all
```

> Tip: installing a custom kernel to the VM
>
> If you wish to install packages from the host filesystem to the VM,
> you can use `vm-put-pkg`. This works both manually in the interactive
> prompt, and in scripts in the `on_vm_online` environment variable.
>
> Example: Install a kernel package, reboot the VM and start the
> interactive prompt once the VM has rebooted. Finally exit the test
> framework when the user quits the interactive prompt.
> ```
> on_vm_online='vm-put-pkg kernel-default-5.15*rpm && vm-reboot && interactive; exit'
> ```

### Install memtierd on the VM

Use `govm ls` on the host to find out the IP address of the VM where
to install `memtierd`

```
scp bin/memtierd opensuse@172.17.0.2:
ssh opensuse@172.17.0.2 "sudo mv memtierd /usr/local/bin"
```

Optional: `meme` is a memory exerciser program, developed for
`memtierd` testing and development. You can build and install it as
follows:

```
make bin/meme
scp bin/meme opensuse@172.17.0.2:
ssh opensuse@172.17.0.2 "sudo mv meme /usr/local/bin"
```

### Use memtierd in the VM

1. Login to the VM (use `govm ls` to find the IP address of the correct VM):
   ```
   ssh opensuse@172.17.0.2
   ```

   Note: all commands below in this section are executed on the VM.

   You can use `numactl` to inspect the topology and free memory on each NUMA node
   ```
   sudo zypper in numactl
   sudo numactl -H
   ```

2. Create a process that uses a lot of memory.

   Example: with the following command `meme` allocates 1 GB of
   memory. The first 128M is only read, next 128M is read and written,
   the next 128M is only written, and the remaining 640M is idle:

   ```
   meme -bs 1G -brs 256M -bws 256M -bwo 128M
   ```

   See `meme -h` for more options.

3. Start memtierd

   Command mode

   ```
   sudo memtierd -prompt
   ```

   Automatic mode

   ```
   sudo memtierd -config FILE
   ```

4. Observe how process's memory is managed.

   - `/proc/PID/numa_maps` includes the number of process's memory pages on each NUMA node.

   - `/sys/fs/cgroup/.../memory.numa_stat` includes the number of
     bytes of memory of all processes in a cgroup (in cgroup v2).

     > Tip: `awk` spells for parsing and summarizing the files above:
     > ```
     > # Total memory of MYPROCESS (note: assuming page size of 4 kB)
     > awk 'BEGIN{RS=" ";FS="="}/N[0-9]+/{mem[$1]+=$2}END{for(node in mem){print node" "mem[node]*4/1024" M"}}' < /proc/$(pidof MYPROCESS)/numa_maps
     >
     > # Anonymous memory of all processes in a cgroup
     > awk 'BEGIN{RS=" ";FS="="}/N[0-9]+/{mem[$1]+=$2}END{for(node in mem){print node" "mem[node]/1024/1024" M"}}' <(grep ^anon /sys/fs/cgroup/.../memory.numa_stat)
     > ```

  - The `stats` command in the `memtierd` prompt reports memory
    scanning times and summarizes amount of memory moved to each
    node.

    ```
    memtierd> stats
    move_pages on pid: 13788
        calls: 488
        requested: 499712 pages (1952 MB)
        on target: 498848 pages (1948 MB)
            to node 0: 149664 pages (584 MB)
            to node 2: 349184 pages (1364 MB)
        errors: 0 pages (0 MB)
    memory scans on pid: 13788
        scans: 8
        scan time: 1066 ms (133 ms/scan)
        scanned: 13633878 pages (1704234 pages/scan, 6657 MB/scan)
        accessed: 0 pages (0 pages/scan, 0 MB/scan)
        written: 3297476 pages (412184 pages/scan, 1610 MB/scan)
    ```

## Policies

Memtierd implements two policies: age and heat. These policies have
different ways of interpreting memory tracker counters as page
activity.

### The age policy

The age policy keeps record on two times:

1. Idle time: how long a time a page has been completely idle.

2. Active time: how long a time a page has been active every time when
   checked.

If the idle time exceeds `IdleDurationMs` in the policy configuration,
the page is moved a node in `IdleNumas` (demotion). If the active time
exceeds `ActiveDurationMs`, the page is moved to a node in
`ActiveNumas` (promotion). Demotion and promotion are disabled if the
corresponding duration equals 0, or if the list of corresponding nodes
is empty.

Example: a page is idle if a tracker has not seen activity in the past
15 seconds. On the other hand, a page is active if tracker has seen
activity on every scan in the past 10 seconds. In both cases pages are
moved.

```
memtierd> policy -create age -config {"Tracker":{"Name":"softdirty","Config":"{\"PagesInRegion\":256,\"MaxCountPerRegion\":1,\"ScanIntervalMs\":4000,\"RegionsUpdateMs\":0,\"SkipPageProb\":0,\"PagemapReadahead\":0}"},"Mover":{"IntervalMs":20,"Bandwidth":200},"Cgroups":["/sys/fs/cgroup/foobar"],"IntervalMs":5000,"IdleDurationMs":15000,"IdleNumas":[2,3],"ActiveDurationMs":10000,"ActiveNumas":[0,1]} -start
```

Currently the age policy works with idlepage and softdirty trackers,
but not with the damon tracker.

### The heat policy

The heat policy stores tracker counters into a heatmap, and moves
pages based on their heat.

The heatmap quantifies heats of pages (heats values from 0.0 to
`HeatMax`) into classes. The number of classes is specified with
`HeatClasses`. The heat classes are named 0, 1, ..., `HeatClasses`-1,
the last being the hottest. `HeatRetention` is the portion of the heat
that retains in the map after one second of inactivity.

The policy parameter `HeatNumas` maps heat classes into sets of NUMA
nodes. A page that belongs to a class should be moved into any NUMA
node associated with this class. If a heat class is missing from the
`HeatNumas` map, a page in that heat class will not be moved.

Example: divide pages into four heat classes: 0, 1, 2 and 3. Move
hottest pages (class 3) to nodes 0 or 1, and coldest pages (class 0)
to 2 or 3, and leave intermediate pages unmoved.

```
memtierd> policy -create heat -config {"Tracker":{"Name":"idlepage","Config":"{\"PagesInRegion\":256,\"MaxCountPerRegion\":0,\"ScanIntervalMs\":5000,\"RegionsUpdateMs\":0,\"PagemapReadahead\":0,\"KpageflagsReadahead\":0,\"BitmapReadahead\":0}"},"Heatmap":{"HeatMax":0.01,"HeatRetention":0.8,"HeatClasses":4},"Mover":{"IntervalMs":20,"Bandwidth":200},"Cgroups":["/sys/fs/cgroup/foobar"],"IntervalMs":10000,"HeatNumas":{"0":[2,3],"3":[0,1]}}
```

The heat policy works with all trackers.

## Trackers

Trackers track memory activity of a set of processes. List of
trackers, what they detect and their dependencies.

- damon:
  - Detects reads and writes.
  - Kernel configuration: `DAMON`, `DAMON_DBGFS` (in 5.15 and later)
  - Userspace interface:
    - `/sys/kernel/debug/damon`
    - The `perf` tool.
- idlepage:
  - Detects reads and writes.
  - Kernel configuration: `IDLE_PAGE_TRACKING`
  - Userspace interface:
    - `/sys/kernel/mm/page_idle/bitmap`
- softdirty:
  - Detect only writes.
  - Kernel configuration: `MEM_SOFT_DIRTY`
  - Userspace interface:
    - `/proc/PID/clear_refs`
    - `/proc/PID/pagemap`
