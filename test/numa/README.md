# CRI Resource Manager - NUMA node tests

## Prerequisites

Install:
- `docker`
- `govm`
  In case of errors in building `govm` with `go get`, or creating a virtual machine (`Error when creating the new VM: repository name must be canonical`), these are the workarounds:
  ```
  cd ~/go/src && go get -d github.com/govm-project/govm && cd github.com/govm-project/govm && go install  && cd .. && docker build govm -f govm/Dockerfile -t govm/govm:latest
  ```

## Usage

```
[VAR=VALUE...] ./run.sh MODE
```

Get help on available `VAR=VALUE`'s with `./run.sh help`.

## Test phases

In the *setup phase* NUMA node tests create a virtual machine unless
it already exists. When it is running, tests create a single-node
cluster and launches `cri-resmgr` on it, unless they are already
running.

In the *test phase* NUMA node tests run the default test script, a
custom script, or give a prompt (`run.sh> `) asking a user to run test
script commands in the `interactive` mode. *Test scripts* are `bash`
scripts that can use helper functions for running commands and
observing the status of the virtual machine and software running on
it.

In the *tear down phase* NUMA node tests copy logs from the virtual
machine and finally stop or delete the virtual machine, if that is
wanted.

## Test modes

- `test` mode runs fast and reports `Test verdict: PASS` or
  `FAIL`. The exit status is zero if and only if a test passed.

- `play` mode runs the same phases and scripts as the `test` mode, but
  slower. This is good for following and demonstrating what is
  happening.

- `interactive` mode runs the setup and tear down phases, but instead
  of executing a test script it gives an interactive prompt.

Print help to see clean up, execution speed and other options for all
modes.

## Running from scratch and quick rerun in existing virtual machine

The test will use `govm`-managed virtual machine named in the `vm`
environment variable. The default is `crirm-test-numa`. If a virtual
machine with that name exists, the test will be run on it. Otherwise
the test will create a virtual machine with that name from
scratch. You can delete a virtual machine with `govm delete NAME`.

If you want rerun the test many times, possibly with different test
inputs or against different versions of `cri-resmgr`, either use the
`play` mode or set `cleanup=0` in order to keep the virtual machine
after each run. Then tests will run in the same single node cluster,
and the test script will only delete running pods before launching new
ones.

## Testing locally built cri-resmgr and cri-resmgr from github

If you make changes to `cri-resmgr` sources and rebuild it, you can
force the test script to reinstall newly built `cri-resmgr` to
existing virtual machine before rerunning the test:

```
cri-resource-manager$ make
cri-resource-manager$ cd test/numa
numa$ reinstall_cri_resmgr=1 speed=1000 ./run.sh play
```

You can also let the test script build `cri-resmgr` from the github
master branch. This takes place inside the virtual machine, so your
local git sources will not be affected:

```
numa$ reinstall_cri_resmgr=1 binsrc=github ./run.sh play
```

## Custom tests

You can run a custom test script in a virtual machine with NUMA
setup and single-node Kubernetes cluster is running. Example:

```
$ cat > myscript.sh << EOF
# create two pods, each requesting two CPUs
CPU=2 n=2 create guaranteed
# create four pods, no resource requests
n=4 create besteffort
# show pods
vm-command "kubectl get pods"
# check that
#   1. the first two pods are not allowed to use the same CPUs
#   2. the first two and the last four pods are not allowed to run on
#      the same CPUs
#   3. the first two pods get four CPUs in total.
verify 'cpus[0] & cpus[1] == 0' \
       '(cpus[2] | cpus[3] | cpus[4] | cpus[5]) & (cpus[0] | cpus[1]) == 0' \
       'bin(cpus[0] | cpus[1]).count("1") == 4'
EOF
$ ./run.sh play myscript.sh
```

Interactive mode helps developing and debugging custom scripts:

```
$ ./run.sh interactive
...
run.sh> CPU=2 n=2 create guaranteed
```

You can get help on functions available in test scripts with `./run.sh
help script`, or with `help` when in the interactive mode.

## Testing different NUMA node configurations

If you change NUMA node topology of an existing virtual machine, you
must delete the virtual machine first. Otherwise the `topology` variable
is ignored and the test will run in the existing NUMA
configuration.

The `topology` variable is a JSON array of objects. Each object
defines one or more NUMA nodes. Keys in objects:
```
"mem"                 mem (RAM) size on each NUMA node in this group.
                      The default is "0G".
"nvmem"               nvmem (non-volatile RAM) size on each NUMA node
                      in this group. The default is "0G".
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
```


Example:

Run the test in a VM with two NUMA nodes. There are 4 CPUs (two cores, two
threads per core by default) and 4G RAM in each node
```
numa$ govm delete my2x4 ; vm=my2x4 topology='[{"mem":"4G","cores":2,"nodes":2}]' ./run.sh play
```

Run the test in a VM with 32 CPUs in total: there are two packages
(sockets) in the system, each containing two dies. Each die containing
two NUMA nodes, each node containing 2 CPU cores, each core containing
two threads. And with a NUMA node with 16G of non-volatile memory
(NVRAM) but no CPUs.

```
numa$ vm=mynvram topology='[{"mem":"4G","cores":2,"nodes":2,"dies":2,"packages":2},{"nvmem":"16G"}]' ./run.sh play
```

## Test output

All test output is saved under the directory in the environment
variable `outdir`. The default is `./output`.

Executed commands with their output, exit status and timestamps are
saved under the `output/commands` directory.

You can find Qemu output from Docker logs. For instance, output of the
most recent Qemu launced by `govm`:
```
$ docker logs $(docker ps | awk '/govm/{print $1; exit}')
```
