# memtierd - utility / daemon for moving memory between NUMA nodes

## Build

```
make bin/memtierd
```

## Test environment

### Install e2e test framework dependencies
1. docker
2. govm

   Example on building govm and govm/govm:latest Docker image on Ubuntu:
   ```
   sudo apt install -y docker.io git-core golang
   export GOPATH=$HOME/go
   export PATH=$PATH:$GOPATH/bin
   GO111MODULE=off go get -d github.com/govm-project/govm && cd $GOPATH/src/github.com/govm-project/govm && go mod tidy && go mod download && go install && cd .. && docker build govm -f govm/Dockerfile -t govm/govm:latest
   ```

### Create a virtual machine with the topology of your interest

Example of a four-NUMA-node topology with:
- 2 x 4 CPU / 4G nodes
- 2 x 2G mem-only nodes

```
topology='[{"cores": 2, "mem": "4G", "nodes": 2}, {"cores":0, "mem":"2G", "nodes": 2}]' distro=opensuse-tumbleweed vm=opensuse-4422 on_vm_online='interactive; exit' test/e2e/run.sh interactive
```

(See supported Linux distributions and other options with
`test/e2e/run.sh help`.)

If you wish to install packages from the host filesystem to the
virtual machine, you can use `vm-put-pkg`. That works in the
interactive prompt, and in the script in the `on_vm_online`
(environment variable) hook. Example: if you wish to install a kernel
package and reboot the virtual machine when becomes online, try above
command with:

```
on_vm_online='vm-put-pkg kernel-default-5.15*rpm && vm-reboot && interactive; exit'
```

You can get help on all available commands in the interactive prompt
and in the scripts:

```
test/e2e/run.sh help script all
```

### Install memtierd on the VM

Use `govm ls` to find out the IP address of the virtual machine where
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

## Use memtierd

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

   If you installed `meme`, run `meme` (see `meme -h` for options).

   As another example, you can create a `python3` process that has 2
   GB of idle memory:

   ```
   python3 -c 'x="x"*(2*1024*1024*1024); input()' &
   ```

3. Start memtierd with interactive prompt.

   ```
   sudo memtierd --prompt
   ```

   (Tip: install `rlwrap` and run `sudo rlwrap memtierd --prompt` to
   enable convenient readline input with history.)

   Run `help` in the `memtierd>` prompt to list commands. On each
   command you can get more help with `COMMAND -h`, for instance
   `pages -h`.

   Example: manage memory locations using the `age` policy. The policy
   moves pages to `IdleNUMA` if the pages have been idle for the last
   `IdleDuration` seconds. And pages that have been active on every
   tracker round for the last `ActiveDuration` seconds are moved to
   `ActiveNUMA`. Those processes will be managed that are found in the
   `Cgroups` list. (Processes in nested groups managed, too.)

   ```
   memtierd> policy -create age -config {"Tracker":"idlepage","Interval":10,"IdleDuration":30,"IdleNUMA":1,"ActiveDuration":10,"ActiveNUMA":0,"Cgroups":["/sys/fs/cgroup/user.slice/mytest"]} -start
   ```

   Policy uses `tracker` and `mover`. For testing and development,
   those can be independently created, configured and used in the
   prompt, too.

   Example: select pages with `pages` and let `mover` work for 10
   seconds moving them to NUMA node 1.

   ```
   ( echo pages -pid $(pidof meme); echo pages; echo mover -pages-to 1; sleep 10; echo pages ) | sudo ./memtierd -prompt
   ```

## Policy: Heat

The heat policy feeds tracker addresses and counter values into a
heatmap, and moves pages based on their heat.

The heatmap quantifies heats of address ranges (heats values [0.0,
`HeatMax`]) into `HeatClasses` classes. The heat classes are named 0,
1, ..., `HeatClasses`-1. `HeatRetention` is the portion of the heat
that retains in the map after one second of inactivity.

The policy parameter `HeatNumas` maps heat classes into sets of NUMA
nodes. A page that belongs to a class should be moved into any NUMA
node in this class. If there is no NUMA node set a heat class, a page
in that heat class will not be moved.

Example where pages are divided into heat classes 0, 1, 2 and 3, and a
page in a class is moved to corresponding NUMA node.

```
memtierd> policy -create heat -config {"Tracker":"idlepage","HeatmapConfig":"{\"HeatMax\":0.01,\"HeatRetention\":0.95,\"HeatClasses\":4}","MoverConfig":"{\"Interval\":10,\"Bandwidth\":100}","Cgroups":["/sys/fs/cgroup/foobar"],"Interval":5,"HeatNumas":{"0":[0],"1":[1],"2":[2],"3":[3]}} -start
```

You can track process memory consumption on each NUMA node by watching

```
while true; do clear; awk 'BEGIN{RS=" ";FS="="}/N[0-9]+/{mem[$1]+=$2}END{for(node in mem){print node" "mem[node]*4/1024" M"}}' < /proc/$(pidof MYPROCESS)/numa_maps; sleep 2; done
```
