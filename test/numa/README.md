# CRI Resource Manager - NUMA node tests

## Prerequisites

Install:
- `docker`
- `govm`
- `jq`

## Usage

```
[VAR=VALUE...] ./run.sh MODE
```

Get help on available `VAR=VALUE`'s with `./run.sh`.

## Two modes: `test` and `play`

`test` mode runs fast and, by default, cleans up everything after test
run. The virtual machine with the contents will be lost. The output
will include `Test verdict:`, and the exit status is zero if and only
if the test passed.

`play` mode runs slower and, by default, leaves the virtual machine
running.

Print help to see clean up, execution speed and other options for both
modes.

## Running from scratch and quick rerun in existing virtual machine

The test will use `govm` virtual machine named in the `vm` environment
variable. The default is `crirm-test-numa`. If a virtual machine with
that name exists, the test will be run on it. Otherwise the test will
create a virtual machine with that name from scratch. You can delete a
virtual machine with `govm delete NAME`.

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

## Testing different Pods

The test will instantiate Pods listed in the `pods` variable in
JSON. You can print the default `pods` with `./run.sh help defaults`.

The JSON list contains pod groups. Each group specifies template file,
number of pods to be created with these specifications, and contents
to the fields in template files (`CPU`, `CPULIM`, `MEM`, ... depends
on the template).

```
{
  "q": "guaranteed", // specifies the QoS class, or more precisely,
                     // the Pod template file (here guaranteed.yaml.in)
                     // from which pods will be instantiated.
  "pods": 3,         // how many pods like this will be created.
                     // The default is 1.
  "CPU": 2,          // how many CPUs each pod will require. Field ${CPU}
                     // in the template will be replaced with this value.
                     // The defaults of template field values is printed by
                     // ./run.sh help defaults.
  "MEM": "3G",       // how much memory each pod will require.
}
```

You can create `mycustom.yaml.in` Pod templates and use those in the
same way as `guaranteed`, `burstable` and `besteffort` templates.

## Testing different NUMA node configurations

If you change NUMA node topology of an existing virtual machine, you
must delete the virtual machine first. Otherwise `numanodes` variable
is ignored and the test will run in the existing NUMA
configuration. Example:

```
numa$ govm delete my2x4
```

Run the test in a VM with two NUMA nodes, 4 CPUs and 4G RAM in each node
```
numa$ govm delete my2x4 ; vm=my2x4 numanodes='[{"cpu":4,"mem":"4G","nodes":2}]' ./run.sh play
```

Run the test in a VM with two NUMA nodes, 8 CPUs and 4G of memory in
one, no CPUs and 16G of non-volatile memory (NVRAM) in the other

```
numa$ vm=mynvram numanodes='[{"cpu":8,"mem":"4G"},{"nvmem":"16G"}]' ./run.sh play
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
