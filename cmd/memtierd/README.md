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
topology='[{"cores": 2, "mem": "4G", "nodes": 2}, {"cores":0, "mem":"2G", "nodes": 2}]' distro=opensuse-tumbleweed vm=opensuse-4422 on_vm_online='exit' test/e2e/run.sh interactive
```

### Install memtierd on the VM

Use `govm ls` to find out the IP address of the virtual machine where to install `memtierd`

```
scp bin/memtierd opensuse@172.17.0.2:
ssh opensuse@172.17.0.2 "sudo mv memtierd /usr/local/bin"
```

## Use memtierd

1. Login to the VM (use `govm ls` to find the IP address of the correct VM):
   ```
   ssh opensuse@172.17.0.2
   ```

   All commands below are executed on the VM.

   You can use `numactl` to inspect the topology and free memory on each NUMA node
   ```
   sudo zypper in numactl
   sudo numactl -H
   ```

2. Create a process that needs a lot of memory.
   For instance, a python3 process that takes 2G.
   ```
   python3 -c 'x="x"*2*1024*1024*1024; input()' &
   ```

3. See the memory status of the process.
   ```
   sudo memtierd --pid=$(pidof python3)
   ```
   Move 100,000 pages from any NUMA node to node 2:
   ```
   sudo memtierd --pid=$(pidof python3) --move-to=2 --count=100000
   ```
   Move 100,000 pages from NUMA node 1 to node 3:
   ```
   sudo memtierd --pid=$(pidof python3) --move-from=1 --move-to=3 --count=100000
   ```
