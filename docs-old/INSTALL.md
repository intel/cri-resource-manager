## Installing From Packages

You can install CRI Resource Manager from `deb` or `rpm` packages
for supported distros.

  - [download](https://github.com/intel/cri-resource-manager/releases) packages
  - install them:
    - for rpm packages: `sudo rpm -Uvh <packages>`
    - for deb packages: `sudo dpkg -i <packages>`

## Installing From Sources

Although not recommended, you can install CRI Resource Manager from sources:

  - get the sources: `git clone https://github.com/intel/cri-resource-manager`
  - build and install: `cd cri-resource-manager; make build && sudo make install`

You will need at least `git`, `golang 1.14` or newer, `GNU make`, `bash`,
`find`, `sed`, `head`, `date`, and `install` to be able to build and install
from sources.

## Building Packages for the Distro of Your Host

You can build packages for the `$distro` of your host by executing the
following command:

```
make packages
```

If the `$version` of your `$distro` is supported, this will leave the
resulting packages in `packages/$distro-$version`. Building packages
this way requires `docker`, but it does not require you to install
the full set of build dependecies of CRI Resource Manager to your host.

If you want to build packages without docker, you can use either
`make rpm` or `make deb`, depending on which supported distro you are
running. Building this way requires all the build dependencies to be
installed on your host.

You can check which `$distro`'s and `$version`'s are supported by running

```
ls dockerfiles/cross-build
```

If you see a `Dockerfile.$distro-$version` matching your host then your
distro is supported.

## Building Packages for Another Distro

You can cross-build packages of the native `$type` for a particular
`$version` of a `$distro` by running the follwing command:

```
make cross-$type.$distro-$version
```

Similarly to `make packages` this will build packages using a `docker`
container. However, instead of building for your host, it will build them
for the specified distro. For instance `make cross-deb.ubuntu-18.04` will
build `deb` packages for `Ubuntu 18.04`, and `make cross-rpm.centos-8` will
build `rpm` packages for `CentOS 8`

## Post-Install Configuration

The provided packages install `systemd` service files and sample configuration.
The easiest way to get up and running is to rename the sample configuration and
start CRI Resource Manager using systemd. You can do this using the following
commands:

```
mv /etc/cri-resmgr/fallback.cfg.sample /etc/cri-resmgr/fallback.cfg
systemctl start cri-resource-manager
```

If you want, you can set up automatic starting of CRI Resource Manager
when your system boots with this command:

```
systemctl enable cri-resource-manager
```

The provided packages also install a file for managing the default options
passed to CRI Resource Manager upon startup. You can change these by editing
this file and then restarting CRI Resource Manager, like this:

```
# On debian-based systems edit the defaults like this:
${EDITOR:-vi} /etc/default/cri-resource-manager
# On rpm-based systems edit the defaults like this:
${EDITOR:-vi} /etc/sysconfig/cri-resource-manager
# Restart the service.
systemctl restart cri-resource-manager
```