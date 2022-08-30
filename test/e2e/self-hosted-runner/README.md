Github self hosted action runner setup
======================================

This package describes how to setup Github action-runner [[1]](#s1) to run in self hosted environment [[2]](#s2).

The setup is devided into two parts, in step 1) a generic Ubuntu 20.04 base image is used as a base
and new software like Go compiler is installed into it. Then in step 2) this base image is used
to run the actual CRI-RM e2e tests in it. A freshly installed host Ubuntu image is used for each
e2e test run.

There are several layer of VMs and host OS here:

* Your host OS can be Fedora or some other Linux distribution. In the examples below,
  the host installation instructions are for Fedora but you can use Ubuntu too.

* In the host OS, we will install vagrant tool that manages the action runner VM. The base OS in VM
  is Ubuntu 20.04. If you want to use some other OS, create new directories in provision/ and runner/
  directories and modify the relevant scripts in those directories as needed.

* The actual e2e tests are run inside the action runner VM. There the e2e scripts use govm tool to
  install the actual VM image used in the testing. The desired e2e OS is selected by the github
  workflow file and is not selected by the action runner scripts.


Details
-------

The base VM image is created in your host where you need to install vagrant, libvirt and relevant tools.

For Fedora, this means you need to install these packages to your host:
```
  $ sudo dnf install vagrant vagrant-libvirt
  $ sudo dnf install bridge-utils
  $ sudo dnf install libvirt /usr/bin/virt-sysprep
  $ sudo dnf groupinstall virtualization
  $ sudo systemctl unmask virtnetworkd-ro.socket
  $ sudo systemctl enable virtnetworkd-ro.socket
  $ sudo systemctl enable --now libvirtd virtlogd
```
For Ubuntu, you need to install these packages to your host:

  TBD

You need to add the self hosted runner to a repository in github. See instructions at [[3]](#s3).
You need to copy the version of the runner, the repository URL and the access token from
the github page into the env configuration file.

Create a configuration file for the runner:
```
  $ cp env.template env
```
Edit the env file, and add relevant parameters from github action runner settings there.
If you are behind a proxy, modify the proxy settings in env file accordingly.

After the configuration phase, we can create the provision base VM image for the self
hosted action-runner like this (all the commands can be done as a normal user):
```
  $ cd provision/ubuntu-20.04
  $ make provision
```
Note that if this Docker permission issue is printed during the provisioning phase above
```
  default: Got permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock: Post "http://%2Fvar%2Frun%2Fdocker.sock/v1.24/build?...&version=1": dial unix /var/run/docker.sock: connect: permission denied
```
then just re-run `make provision`. The reason for the Docker error is that the `vagrant` user needs to
logout and then login back to VM in order it to be added to the `docker` group properly. This login/logout
is currently not automated and needs to be done manually.

Then we need to repackage the generated VM so that we can use it when running the e2e tests.
```
  $ make package
```
This will generate new vagrant base box image `../../output/provisioned-ubuntu2004.box` that is used
to create a scratch VM where the actual e2e test script is run.
Usually this base box image needs only be re-created if new versions of the installed packages need to be
used, like when golang version changes or some security issue needs to be fixed.

After the package is created, the new box image needs to be taken into use. First we need to add
the generated box image into the system.
```
  $ cd ../..
  $ vagrant box add --name ubuntu-20.04/runner --provider libvirt output/provisioned-ubuntu2004.box
```
Note that you can move this generated box image from the output directory to another host if needed,
as it is self contained and can be installed in different system than where the provisioning took place.

If you have re-generated the provision image, please note that there might be a cached image in
`~/.local/share/libvirt/images` directory that you should remove before runner.sh creates the new
runner image. So do this in that situation:
```
  $ cd runner/ubuntu-20.04
  $ make cleanup
```
If the libvirt environment needs cleanup too, then execute these commands as a root user:
```
  # virsh list --all
  # virsh destroy <THE_MACHINE>
  # virsh undefine <THE_MACHINE> --snapshots-metadata --managed-save
  # virsh vol-list default
  # virsh vol-delete --pool default <THE_VOLUME>
```
Final step is to configure and run the self-hosted action runner. Note that at this point, the
runner will contact github so the URL and token needs to be set properly in the env configuration
file.
```
  $ cd runner/ubuntu-20.04
  $ ./runner.sh
```
The `runner.sh` will start/create the runner VM and execute the actual self-hosted runner script (`run.sh`)
that is provided by the github runner source package. The `run.sh` script will then wait for any jobs from
the repository you have configured.

When a runner job has finished i.e., the e2e tests have been executed, the results can be seen in github
actions page. The self-hosted action runner VM in destroyed after the job, and new and fresh VM is
created to be ready to serve new job requests.

[1]<a name="s1"></a> https://docs.github.com/en/rest/actions

[2]<a name="s2"></a> https://docs.github.com/en/rest/actions/self-hosted-runners

[3]<a name="s3"></a> https://docs.github.com/en/actions/hosting-your-own-runners/adding-self-hosted-runners#adding-a-self-hosted-runner-to-a-repository
