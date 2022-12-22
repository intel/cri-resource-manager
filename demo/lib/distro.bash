# shellcheck disable=SC2120
GO_URLDIR=https://golang.org/dl
GO_VERSION=1.19.1
GOLANG_URL=$GO_URLDIR/go$GO_VERSION.linux-amd64.tar.gz
CRICTL_VERSION=${CRICTL_VERSION:-"v1.25.0"}
MINIKUBE_VERSION=${MINIKUBE_VERSION:-v1.27.0}

###########################################################################

#
# distro-agnostic interface
#
# To add a new distro implement distro-specific versions of these
# functions. You can omit implementing those which already resolve
# to an existing function which works for the new distro.
#
# To add a new API function, add an new briding resolution entry below.
#

distro-image-url()          { distro-resolve "$@"; }
distro-ssh-user()           { distro-resolve "$@"; }
distro-pkg-type()           { distro-resolve "$@"; }
distro-install-repo-key()   { distro-resolve "$@"; }
distro-install-repo()       { distro-resolve "$@"; }
distro-refresh-pkg-db()     { distro-resolve "$@"; }
distro-install-pkg()        { distro-resolve "$@"; }
distro-install-pkg-local()  { distro-resolve "$@"; }
distro-remove-pkg()         { distro-resolve "$@"; }
distro-setup-proxies()      { distro-resolve "$@"; }
distro-setup-oneshot()      { distro-resolve "$@"; }
distro-install-utils()      { distro-resolve "$@"; }
distro-install-golang()     { distro-resolve "$@"; }
distro-install-runc()       { distro-resolve "$@"; }
distro-install-containerd() { distro-resolve "$@"; }
distro-config-containerd()  { distro-resolve "$@"; }
distro-restart-containerd() { distro-resolve "$@"; }
distro-install-crio()       { distro-resolve "$@"; }
distro-config-crio()        { distro-resolve "$@"; }
distro-restart-crio()       { distro-resolve "$@"; }
distro-install-crictl()     { distro-resolve "$@"; }
distro-install-cri-dockerd(){ distro-resolve "$@"; }
distro-install-minikube()   { distro-resolve "$@"; }
distro-install-k8s()        { distro-resolve "$@"; }
distro-install-kernel-dev() { distro-resolve "$@"; }
distro-k8s-cni()            { distro-resolve "$@"; }
distro-k8s-cni-subnet()     { distro-resolve "$@"; }
distro-set-kernel-cmdline() { distro-resolve "$@"; }
distro-govm-env()           { distro-resolve "$@"; }
distro-bootstrap-commands() { distro-resolve "$@"; }
distro-env-file-dir()       { distro-resolve "$@"; }

###########################################################################

# distro-specific function resolution
distro-resolve() {
    local apifn="${FUNCNAME[1]}" fn prefn postfn
    # shellcheck disable=SC2086
    {
        fn="$(distro-resolve-fn $apifn)"
        prefn="$(distro-resolve-fn $apifn-pre)"
        postfn="$(distro-resolve-fn $apifn-post)"
        command-debug-log "$VM_DISTRO/${FUNCNAME[1]}: pre: ${prefn:--}, fn: ${fn:--}, post: ${postfn:--}"
    }
    [ -n "$prefn" ] && { $prefn "$@" || return $?; }
    $fn "$@" || return $?
    [ -n "$postfn" ] && { $postfn "$@" || return $?; }
    return 0
}

distro-resolve-fn() {
    # We try resolving distro-agnostic implementations by looping through
    # a list of candidate function names in decreasing order of precedence
    # and returning the first one found. The candidate list has version-
    # exact and unversioned distro-specific functions and a set fallbacks
    # based on known distro, derivative, and package type relations.
    #
    # For normal functions the last fallback is 'distro-unresolved' which
    # prints and returns an error. For pre- and post-functions there is no
    # similar setup. IOW, unresolved normal distro functions fail while
    # unresolved pre- and post-functions get ignored (in distro-resolve).
    local apifn="$1" candidates fn

    case $apifn in
        distro-*) apifn="${apifn#distro-}";;
        *) error "internal error: can't resolve non-API function $apifn";;
    esac
    candidates="${VM_DISTRO/./_}-$apifn ${VM_DISTRO%%-*}-$apifn"
    case $VM_DISTRO in
        ubuntu*) candidates="$candidates debian-$apifn";;
        centos*) candidates="$candidates fedora-$apifn rpm-$apifn";;
        fedora*) candidates="$candidates rpm-$apifn";;
        *suse*)  candidates="$candidates rpm-$apifn";;
        sles*)   candidates="$candidates opensuse-$apifn rpm-$apifn";;
    esac
    case $apifn in
        *-pre|*-post) ;;
        *) candidates="$candidates default-$apifn distro-unresolved";;
    esac
    for fn in $candidates; do
        if [ "$(type -t -- "$fn")" = "function" ]; then
            echo "$fn"
            return 0
        fi
    done
}

# distro-unresolved terminates failed API function resolution with an error.
distro-unresolved() {
    local apifn="${FUNCNAME[2]}"
    command-error "internal error: can't resolve \"$apifn\" for \"$VM_DISTRO\""
    return 1
}

###########################################################################

#
# Ubuntu 18.04, 20.04, Debian 10, generic debian
#

ubuntu-18_04-image-url() {
    echo "https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img"
}

ubuntu-20_04-image-url() {
    echo "https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
}

ubuntu-22_04-image-url() {
    echo "https://cloud-images.ubuntu.com/releases/jammy/release/ubuntu-22.04-server-cloudimg-amd64.img"
}

debian-10-image-url() {
    echo "https://cloud.debian.org/images/cloud/buster/20200803-347/debian-10-generic-amd64-20200803-347.qcow2"
}

debian-11-image-url() {
    echo "https://cloud.debian.org/images/cloud/bullseye/daily/latest/debian-11-generic-amd64-daily.qcow2"
}

debian-sid-image-url() {
    echo "https://cloud.debian.org/images/cloud/sid/daily/latest/debian-sid-generic-amd64-daily.qcow2"
}

ubuntu-download-kernel() {
    # Usage:
    #   ubuntu-download-kernel list
    #   ubuntu-download-kernel VERSION
    #
    # List or download Ubuntu kernel team kernels.
    #
    # Example:
    #   ubuntu-download-kernel list | grep 5.9
    #   ubuntu-download-kernel 5.9-rc8
    #   vm-command "dpkg -i kernels/linux*rc8*deb"
    #   vm-reboot
    #   vm-command "uname -a"
    local version=$1
    [ -n "$version" ] ||
        error "missing kernel version to install"
    if [ "$version" == "list" ]; then
        wget -q -O- https://kernel.ubuntu.com/~kernel-ppa/mainline/  | grep -E '^<tr>.*href="v[5-9]' | sed 's|^.*href="v\([0-9][^"]*\)/".*$|\1|g'
        return 0
    fi
    vm-command "mkdir -p kernels; rm -f kernels/linux*$version*deb; for deb in \$(wget -q -O- https://kernel.ubuntu.com/~kernel-ppa/mainline/v$version/ | awk -F'\"' '/amd64.*deb/{print \$2}' | grep -v -E 'headers|lowlatency'); do ( cd kernels; wget -q https://kernel.ubuntu.com/~kernel-ppa/mainline/v$version/\$deb ); done; echo; echo 'Downloaded kernel packages:'; du -h kernels/*.deb" ||
        command-error "downloading kernel $version failed"
}

ubuntu-ssh-user() {
    echo ubuntu
}

debian-ssh-user() {
    echo debian
}

debian-pkg-type() {
    echo deb
}

debian-install-repo-key() {
    local key
    # apt-key needs gnupg2, that might not be available by default
    vm-command "command -v gpg >/dev/null 2>&1" || {
        vm-command "apt-get update && apt-get install -y gnupg2"
    }
    for key in "$@"; do
        vm-command "curl -L -s $key | apt-key add -" ||
            command-error "failed to install repo key $key"
    done
}

debian-install-repo() {
    if [ $# = 1 ]; then
        # shellcheck disable=SC2086,SC2048
        set -- $*
    fi
    vm-command "echo $* > /etc/apt/sources.list.d/$3-$4.list && apt-get update" ||
        command-error "failed to install apt repository $*"
}

debian-refresh-pkg-db() {
    vm-command "apt-get update" ||
        command-error "failed to refresh apt package DB"
}

debian-install-pkg() {
    # dpkg configure may ask "The default action is to keep your
    # current version", for instance when a test has added
    # /etc/containerd/config.toml and then apt-get installs
    # containerd. 'yes ""' will continue with the default answer (N:
    # keep existing) in this case. Without 'yes' installation fails.

    # Add apt-get option "--reinstall" if any environment variable
    # reinstall_<pkg>=1
    local pkg
    local opts=""
    for pkg in "$@"; do
        if [ "$(eval echo \$reinstall_$pkg)" == "1" ]; then
            opts="$opts --reinstall"
            break
        fi
    done
    vm-command "yes \"\" | DEBIAN_FRONTEND=noninteractive apt-get install $opts -y --allow-downgrades $*" ||
        command-error "failed to install $*"
}

debian-remove-pkg() {
    vm-command "for pkg in $*; do dpkg -l \$pkg >& /dev/null && apt remove -y --purge \$pkg || :; done" ||
        command-error "failed to remove package(s) $*"
}

debian-install-pkg-local() {
    local force=""
    if [ "$1" == "--force" ]; then
        force="--force-all"
        shift
    fi
    vm-command "dpkg -i $force $*" ||
        command-error "failed to install local package(s)"
}

debian-install-golang() {
    debian-refresh-pkg-db
    debian-install-pkg golang git-core
}

debian-install-kernel-dev() {
    distro-refresh-pkg-db
    distro-install-pkg git-core build-essential linux-source bc kmod cpio flex libncurses5-dev libelf-dev libssl-dev dwarves bison
    vm-command "[ -d linux ] || git clone https://github.com/torvalds/linux"
    vm-command '[ -f linux/.config ] || cp -v /boot/config-$(uname -r) linux/.config'
    echo "Kernel ready for patching and configuring."
    echo "build:   cd linux && make bindeb-pkg"
    echo "install: dpkg -i linux-*.deb"
}

debian-10-install-containerd-pre() {
    debian-install-repo-key https://download.docker.com/linux/debian/gpg
    debian-install-repo "deb https://download.docker.com/linux/debian buster stable"

}

debian-sid-install-containerd-post() {
    vm-command "sed -e 's|bin_dir = \"/usr/lib/cni\"|bin_dir = \"/opt/cni/bin\"|g' -i /etc/containerd/config.toml"
}

debian-install-cri-dockerd-pre() {
    debian-refresh-pkg-db
    debian-install-pkg docker.io conntrack
    vm-command "addgroup $(vm-ssh-user) docker"
    distro-install-golang
}

debian-install-crio-pre() {
    debian-refresh-pkg-db
    debian-install-pkg libgpgme11 conmon runc containernetworking-plugins conntrack || true
}

debian-install-k8s() {
    local k8sverparam
    debian-refresh-pkg-db
    debian-install-pkg apt-transport-https curl
    debian-install-repo-key "https://packages.cloud.google.com/apt/doc/apt-key.gpg"
    debian-install-repo "deb https://apt.kubernetes.io/ kubernetes-xenial main"
    if [ -n "$k8s" ]; then
        k8sverparam="=${k8s}-00"
    else
        k8sverparam=""
    fi
    debian-install-pkg "kubeadm$k8sverparam" "kubelet$k8sverparam" "kubectl$k8sverparam"
}

debian-set-kernel-cmdline() {
    local e2e_defaults="$*"
    vm-command "echo 'GRUB_CMDLINE_LINUX_DEFAULT=\"\${GRUB_CMDLINE_LINUX_DEFAULT} ${e2e_defaults}\"' > /etc/default/grub.d/60-e2e-defaults.cfg" || {
        command-error "writing new command line parameters failed"
    }
    vm-command "update-grub" || {
        command-error "updating grub failed"
    }
}

debian-env-file-dir() {
    echo "/etc/default"
}

###########################################################################

#
# Centos 7, 8, generic Fedora
#

YUM_INSTALL="yum install --disableplugin=fastestmirror -y"
YUM_REMOVE="yum remove --disableplugin=fastestmirror -y"

centos-7-image-url() {
    echo "https://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud-2003.qcow2.xz"
}

centos-8-image-url() {
    echo "https://cloud.centos.org/centos/8/x86_64/images/CentOS-8-GenericCloud-8.2.2004-20200611.2.x86_64.qcow2"
}

centos-ssh-user() {
    echo centos
}

centos-7-install-utils() {
    distro-install-pkg /usr/bin/killall
}

centos-7-install-repo() {
    vm-command-q "type -t yum-config-manager >&/dev/null" || {
        distro-install-pkg yum-utils
    }
    vm-command "yum-config-manager --add-repo $*" ||
        command-error "failed to add YUM repository $*"
}

centos-7-install-pkg() {
    vm-command "$YUM_INSTALL $*" ||
        command-error "failed to install $*"
}

centos-7-remove-pkg() {
    vm-command "$YUM_REMOVE $*" ||
        command-error "failed to remove package(s) $*"
}

centos-7-install-containerd-pre() {
    create-ext4-var-lib-containerd
    distro-install-repo https://download.docker.com/linux/centos/docker-ce.repo
}

centos-8-install-pkg-pre() {
    vm-command "[ -f /etc/yum.repos.d/.fixup ]" && return 0
    vm-command 'sed -i "s/mirrorlist/#mirrorlist/g" /etc/yum.repos.d/CentOS-*' && \
    vm-command 'sed -i "s|#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g" /etc/yum.repos.d/CentOS-*' && \
    vm-command 'touch /etc/yum.repos.d/.fixup'
}

centos-8-install-crio-pre() {
    if [ -z "$crio_src" ]; then
        local os=CentOS_8
        local version=${crio_version:-1.20}
        vm-command "curl -L -o /etc/yum.repos.d/libcontainers-stable.repo https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$os/devel:kubic:libcontainers:stable.repo"
        vm-command "curl -L -o /etc/yum.repos.d/crio.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:$version/$os/devel:kubic:libcontainers:stable:cri-o:$version.repo"
    fi
}

centos-8-install-crio() {
    if [ -n "$crio_src" ]; then
	default-install-crio
    else
	distro-install-pkg cri-o
	vm-command "systemctl enable crio"
    fi
}

centos-8-install-containerd-pre() {
    distro-install-repo https://download.docker.com/linux/centos/docker-ce.repo
}

centos-7-k8s-cni() {
    echo "weavenet"
}

centos-install-golang() {
    distro-install-pkg wget tar gzip git-core
    from-tarball-install-golang
}

fedora-image-url() {
    fedora-36-image-url
}

fedora-36-image-url() {
    echo "https://mirrors.xtom.de/fedora/releases/36/Cloud/x86_64/images/Fedora-Cloud-Base-36-1.5.x86_64.qcow2"
}

fedora-35-image-url() {
    echo "https://mirrors.xtom.de/fedora/releases/35/Cloud/x86_64/images/Fedora-Cloud-Base-35-1.2.x86_64.qcow2"
}

fedora-34-image-url() {
    echo "https://mirrors.xtom.de/fedora/releases/34/Cloud/x86_64/images/Fedora-Cloud-Base-34-1.2.x86_64.qcow2"
}

fedora-33-image-url() {
    echo "https://mirrors.xtom.de/fedora/releases/33/Cloud/x86_64/images/Fedora-Cloud-Base-33-1.2.x86_64.qcow2"
}

fedora-ssh-user() {
    echo fedora
}

fedora-install-utils() {
    distro-install-pkg /usr/bin/pidof
}

fedora-install-repo() {
    distro-install-pkg dnf-plugins-core
    vm-command "dnf config-manager --add-repo $*" ||
        command-error "failed to install DNF repository $*"
}

fedora-install-pkg() {
    local pkg
    local do_reinstall=0
    for pkg in "$@"; do
        if [ "$(eval echo \$reinstall_$pkg)" == "1" ]; then
            do_reinstall=1
            break
        fi
    done
    vm-command "dnf install -y $*" ||
        command-error "failed to install $*"
    # When requesting reinstallation, detect which packages were
    # already installed and reinstall those.
    # (Unlike apt and zypper, dnf offers no option for reinstalling
    # existing and installing new packages on the same run.)
    if [ "$do_reinstall" == "1" ]; then
        local reinstall_pkgs
        reinstall_pkgs=$(awk -F '[ -]' -v ORS=" " '/Package .* already installed/{print $2}' <<< "$COMMAND_OUTPUT")
        if [ -n "$reinstall_pkgs" ]; then
            vm-command "dnf reinstall -y $reinstall_pkgs"
        fi
    fi
}

fedora-remove-pkg() {
    vm-command "dnf remove -y $*" ||
        command-error "failed to remove package(s) $*"
}

fedora-install-pkg-local() {
    local force=""
    if [ "$1" == "--force" ]; then
        force="--nodeps --force"
        shift
    fi
    vm-command "rpm -Uvh $force $*" ||
        command-error "failed to install local package(s)"
}

fedora-install-kernel-dev() {
    fedora-install-pkg fedpkg fedora-packager rpmdevtools ncurses-devel pesign grubby git-core
    vm-command "(set -x -e
      echo root >> /etc/pesign/users
      echo $(vm-ssh-user) >> /etc/pesign/users
      /usr/libexec/pesign/pesign-authorize
      fedpkg clone -a kernel
      cd kernel
      git fetch
      git switch ${VM_DISTRO/edora-/} # example: git switch f35 in fedora-35
      sed -i 's/# define buildid .local/%define buildid .e2e/g' kernel.spec
    )" || {
        echo "installing kernel development environment failed"
        return 1
    }
    echo "Kernel ready for patching and configuring."
    echo "build:   cd kernel && dnf builddep -y kernel.spec && fedpkg local"
    echo "install: cd kernel/x86_64 && dnf install -y --nogpgcheck kernel-{core-,modules-,}[5-9]*.e2e.fc*.x86_64.rpm"
}

fedora-install-golang() {
    fedora-install-pkg wget tar gzip git-core
    from-tarball-install-golang
}

fedora-install-crio-version() {
    distro-install-pkg runc conmon
    vm-command "ln -sf /usr/lib64/libdevmapper.so.1.02 /usr/lib64/libdevmapper.so.1.02.1" || true

    if [ -z "$crio_src" ]; then
        vm-command "dnf -y module enable cri-o:${crio_version:-$1}"
    fi
}

fedora-install-containernetworking-plugins() {
    distro-install-pkg containernetworking-plugins
    vm-command "[ -x /opt/cni/bin/loopback ] || { mkdir -p /opt/cni/bin; mount --bind /usr/libexec/cni /opt/cni/bin; }"
    vm-command "grep /usr/libexec/cni /etc/fstab || echo /usr/libexec/cni /opt/cni/bin none defaults,bind,nofail 0 0 >> /etc/fstab"
}

fedora-install-cri-dockerd-pre() {
    distro-install-pkg docker git-core conntrack
    vm-command "systemctl enable docker --now; usermod --append --groups docker $(vm-ssh-user)"
    distro-install-golang
}

fedora-install-crio-pre() {
    fedora-install-crio-version 1.21
    fedora-install-containernetworking-plugins
}

fedora-install-crio() {
    if [ -n "$crio_src" ]; then
        default-install-crio
    else
        distro-install-pkg cri-o
        vm-command "systemctl enable --now crio" ||
            command-error "failed to enable cri-o"
    fi
}

fedora-install-containerd-pre() {
    distro-install-repo https://download.docker.com/linux/fedora/docker-ce.repo
    fedora-install-containernetworking-plugins
}

fedora-install-containerd-post() {
    vm-command "systemctl enable containerd"
}

fedora-install-k8s() {
    local repo="/etc/yum.repos.d/kubernetes.repo"
    local base="https://packages.cloud.google.com/yum/repos/kubernetes-el7-\$basearch"
    local yumkey="https://packages.cloud.google.com/yum/doc/yum-key.gpg"
    local rpmkey="https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg"

    cat <<EOF |
[kubernetes]
name=Kubernetes
baseurl=$base
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=$yumkey $rpmkey
EOF
      vm-pipe-to-file $repo

    if [ -n "$k8s" ]; then
        k8sverparam="-${k8s}-0"
    else
        k8sverparam=""
    fi

    vm-command 'grep -iq centos-[78] /etc/os-release' && \
        vm-command "sed -i 's/gpgcheck=1/gpgcheck=0/g' $repo"

    distro-install-pkg iproute-tc kubelet$k8sverparam kubeadm$k8sverparam kubectl$k8sverparam
    vm-command "systemctl enable --now kubelet" ||
        command-error "failed to enable kubelet"
}

fedora-bootstrap-commands-post() {
    cat <<EOF
reboot_needed=0
mkdir -p /etc/sudoers.d
echo 'Defaults !requiretty' > /etc/sudoers.d/10-norequiretty

setenforce 0
sed -E -i 's/^SELINUX=.*$/SELINUX=permissive/' /etc/selinux/config

echo PATH='\$PATH:/usr/local/bin:/usr/local/sbin' > /etc/profile.d/usr-local-path.sh
EOF
    if [[ "${cgroups:-}" != "v2" ]]; then
        cat <<EOF
if grep -q NAME=Fedora /etc/os-release; then
    if ! grep -q systemd.unified_cgroup_hierarchy=0 /proc/cmdline; then
        sudo grubby --update-kernel=ALL --args="systemd.unified_cgroup_hierarchy=0"
        reboot_needed=1
    fi
fi
EOF
    fi

    # Using swapoff is not enough as we also need to disable the swap from systemd
    # and then reboot the VM
    cat <<EOF
if swapon --show | grep -q partition; then
    sed -E -i '/^\\/.*[[:space:]]swap[[:space:]].*\$/d' /etc/fstab
    systemctl --type swap
    for swp in \`systemctl --type swap | awk '/\\.swap/ { print \$1 }'\`; do systemctl stop "\$swp"; systemctl mask "\$swp"; done
    swapoff --all
    reboot_needed=1
fi
EOF

    cat <<EOF
if [ "\$reboot_needed" == "1" ]; then
   shutdown -r now
fi
EOF
}

fedora-set-kernel-cmdline() {
    local e2e_defaults="$*"
    vm-command "mkdir -p /etc/default; touch /etc/default/grub; sed -i '/e2e:fedora-set-kernel-cmdline/d' /etc/default/grub"
    vm-command "echo 'GRUB_CMDLINE_LINUX_DEFAULT=\"\${GRUB_CMDLINE_LINUX_DEFAULT} ${e2e_defaults}\" # by e2e:fedora-set-kernel-cmdline' >> /etc/default/grub" || {
        command-error "writing new command line parameters failed"
    }
    vm-command "grub2-mkconfig -o /boot/grub2/grub.cfg" || {
        command-error "updating grub failed"
    }
}

fedora-33-install-crio-pre() {
    fedora-install-crio-version 1.20
}

###########################################################################

#
# OpenSUSE and SLES
#

ZYPPER="zypper --non-interactive --no-gpg-checks"

sles-image-url() {
    echo "/DOWNLOAD-MANUALLY-TO-HOME/vms/images/SLES15-SP3-JeOS.x86_64-15.3-OpenStack-Cloud-GM.qcow2"
}

sles-ssh-user() {
    echo "sles"
}

sles-install-utils() {
    local sles_registered=0
    local sles_version=""
    vm-command "SUSEConnect -s" || {
        command-error "cannot run SUSEConnect"
    }
    # Parse registration status and SLES version.
    if [ "$(jq '.[] | select(.identifier == "SLES") | .status' <<< $COMMAND_OUTPUT)" == '"Registered"' ]; then
        sles_registered=1
    fi
    sles_version="$(jq -r '.[] | select(.identifier == "SLES") | .version' <<< $COMMAND_OUTPUT)"
    if [ -z "$sles_version" ]; then
        command-error "cannot read SLES version information from SUSEConnect -s output"
    fi
    # Try automatic registration if registration code is provided.
    if [ "$sles_registered" == 0 ] && [ -n "$VM_SLES_REGCODE" ]; then
            vm-command "SUSEConnect -r $VM_SLES_REGCODE" || {
                echo "ERROR:"
                echo "ERROR: Registering to SUSE Customer Center failed."
                echo "ERROR: - Verify VM_SLES_REGCODE and try again."
                echo "ERROR: - Unset VM_SLES_REGCODE to skip registration (use unsupported repos)."
                echo "ERROR:"
                exit 1
            }
            sles_registered=1
    fi
    # Add correct repo, depending on registration status.
    if [ "$sles_registered" == 0 ]; then
        echo "WARNING:"
        echo "WARNING: Unregistered SUSE Linux Enterprise Server."
        echo "WARNING: VM_SLES_REGCODE is not set, automatic registration skipped."
        echo "WARNING: Fallback to use OpenSUSE OSS repository."
        echo "WARNING:"
        sleep "${warning_delay:-0}"
        vm-command-q "$ZYPPER lr openSUSE-Oss >/dev/null" || {
            distro-install-repo "http://download.opensuse.org/distribution/leap/${sles_version}/repo/oss/" openSUSE-Oss
        }
    else
        vm-command-q "$ZYPPER lr | grep -q SUSE-PackageHub" || {
            vm-command "SUSEConnect -p PackageHub/${sles_version}/x86_64"
        }
    fi
    distro-install-pkg sysvinit-tools psmisc
}

opensuse-image-url() {
    opensuse-15_4-image-url
}

opensuse-15_4-image-url() {
    echo "https://download.opensuse.org/pub/opensuse/distribution/leap/15.4/appliances/openSUSE-Leap-15.4-JeOS.x86_64-15.4-OpenStack-Cloud-Current.qcow2"
}

opensuse-tumbleweed-image-url() {
    echo "https://ftp.uni-erlangen.de/opensuse/tumbleweed/appliances/openSUSE-MicroOS.x86_64-ContainerHost-OpenStack-Cloud.qcow2"
}

opensuse-install-utils() {
    distro-install-pkg psmisc sysvinit-tools
}

opensuse-ssh-user() {
    echo "opensuse"
}

opensuse-pkg-type() {
    echo "rpm"
}

opensuse-set-kernel-cmdline() {
    local e2e_defaults="$*"
    vm-command "mkdir -p /etc/default; touch /etc/default/grub; sed -i '/e2e:opensuse-set-kernel-cmdline/d' /etc/default/grub"
    vm-command "echo 'GRUB_CMDLINE_LINUX_DEFAULT=\"\${GRUB_CMDLINE_LINUX_DEFAULT} ${e2e_defaults}\" # by e2e:opensuse-set-kernel-cmdline' >> /etc/default/grub" || {
        command-error "writing new command line parameters failed"
    }
    vm-command "grub2-mkconfig -o /boot/grub2/grub.cfg" || {
        command-error "updating grub failed"
    }
}

opensuse-setup-oneshot() {
    # Remove bad version of containerd if it is already installed,
    # otherwise valid version of the package will not be installed.
    vm-command "rpm -q containerd && ( zypper info containerd | awk '/Repository/{print $3}' | grep -v Virtualization ) && echo Removing wrong containerd version && zypper --non-interactive rm containerd"
}

opensuse-install-repo() {
    opensuse-wait-for-zypper
    vm-command "$ZYPPER addrepo $* && $ZYPPER refresh" ||
        command-error "failed to add zypper repository $*"
}

opensuse-refresh-pkg-db() {
    opensuse-wait-for-zypper
    vm-command "$ZYPPER refresh" ||
        command-error "failed to refresh zypper package DB"
}

opensuse-install-pkg() {
    opensuse-wait-for-zypper
    # Add zypper option "--force" if environment variable reinstall_<pkg>=1
    local pkg
    local opts=""
    for pkg in "$@"; do
        if [ "$(eval echo \$reinstall_$pkg)" == "1" ]; then
            opts="$opts --force"
            break
        fi
    done
    # In OpenSUSE 15.2 zypper exits with status 106 if already installed,
    # in 15.3 the exit status is 0. Do not consider "already installed"
    # as an error.
    vm-command "$ZYPPER install $opts $*" || [ "$COMMAND_STATUS" == "106" ] ||
        command-error "failed to install $*"
}

opensuse-install-pkg-local() {
    opensuse-wait-for-zypper
    local force=""
    if [ "$1" == "--force" ]; then
        force="--nodeps --force"
        shift
    fi
    vm-command "rpm -Uvh $force $*" ||
        command-error "failed to install local package(s)"
}

opensuse-remove-pkg() {
    vm-command 'for i in $*; do rpm -q --quiet $i || continue; $ZYPPER remove $i || exit 1; done' ||
        command-error "failed to remove package(s) $*"
}

opensuse-install-golang() {
    distro-install-pkg wget tar gzip git-core
    from-tarball-install-golang
}

opensuse-wait-for-zypper() {
    vm-run-until --timeout 5 '( ! pgrep zypper >/dev/null ) || ( pkill -9 zypper; sleep 1; exit 1 )' ||
        error "Failed to stop zypper running in the background"
}

opensuse-require-repo-virtualization-containers() {
    vm-command "zypper ls"
    if ! grep -q Virtualization_containers <<< "$COMMAND_OUTPUT"; then
        opensuse-install-repo https://download.opensuse.org/repositories/Virtualization:containers/15.4/Virtualization:containers.repo
        opensuse-refresh-pkg-db
    fi
}

opensuse-install-crio-pre() {
    opensuse-require-repo-virtualization-containers
    distro-install-pkg --from Virtualization_containers runc conmon
    vm-command "ln -sf /usr/lib64/libdevmapper.so.1.02 /usr/lib64/libdevmapper.so.1.02.1" || true
}

opensuse-install-runc() {
    opensuse-require-repo-virtualization-containers
    distro-install-pkg --from Virtualization_containers runc
}

opensuse-install-containerd() {
    opensuse-require-repo-virtualization-containers
    distro-install-pkg --from Virtualization_containers containerd containerd-ctr
    vm-command "ln -sf /usr/sbin/containerd-ctr /usr/sbin/ctr"

cat <<EOF |
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/sbin/containerd

Delegate=yes
KillMode=process
Restart=always
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=1048576
TasksMax=infinity

[Install]
WantedBy=multi-user.target
EOF
    vm-pipe-to-file /etc/systemd/system/containerd.service

    cat <<EOF |
disabled_plugins = []
EOF
    vm-pipe-to-file /etc/containerd/config.toml
    vm-command "systemctl daemon-reload" ||
        command-error "failed to reload systemd daemon"
}

opensuse-install-k8s() {
    vm-command "( lsmod | grep -q br_netfilter ) || { echo br_netfilter > /etc/modules-load.d/50-br_netfilter.conf; modprobe br_netfilter; }"
    vm-command "echo 1 > /proc/sys/net/ipv4/ip_forward"
    vm-command "zypper ls"
    if ! grep -q snappy <<< "$COMMAND_OUTPUT"; then
        distro-install-repo "http://download.opensuse.org/repositories/system:/snappy/openSUSE_Leap_15.4 snappy"
        distro-refresh-pkg-db
    fi
    distro-install-pkg "snapd apparmor-profiles socat ebtables cri-tools conntrackd iptables ethtool"
    vm-install-containernetworking
    # In some snap packages snap-seccomp launching fails on bad path:
    # cannot obtain snap-seccomp version information: fork/exec /usr/libexec/snapd/snap-seccomp: no such file or directory
    # But snap-seccomp may be installed to /usr/lib/snapd/snap-seccomp.
    # (Found in opensuse-tumbleweed/20210921.)
    # Workaround this problem by making sure that /usr/libexec/snapd/snap-seccomp is found.
    vm-command-q "[ ! -d /usr/libexec/snapd ] && [ -f /usr/lib/snapd/snap-seccomp ]" &&
        vm-command "ln -s /usr/lib/snapd /usr/libexec/snapd"

    vm-command "systemctl enable --now snapd"
    vm-command "snap wait system seed.loaded"
    for kubepart in kubelet kubectl kubeadm; do
        local snapcmd=install
        local k8sverparam
        if vm-command-q "snap info $kubepart | grep -q tracking"; then
            # $kubepart is already installed, either refresh or reinstall it.
            if [ "$(eval echo \$reinstall_$kubepart)" == "1" ]; then
                # Reinstalling $kubepart requested.
                # snap has no option for direct reinstalling,
                # so the package needs to be removed first.
                vm-command "snap remove $kubepart"
                snapcmd=install
            else
                snapcmd=refresh
            fi
        fi
        # Specify snap channel if user has requested a specific k8s version.
        if [[ "$k8s" == *.*.* ]]; then
            echo "WARNING: cannot snap install k8s=X.Y.Z, installing latest X.Y"
            k8sverparam="--channel ${k8s%.*}/edge"
        elif [[ "$k8s" == *.* ]]; then
            k8sverparam="--channel ${k8s}/edge"
        elif [[ -z "$k8s" ]]; then
            k8sverparam=""
        else
            error "invalid k8s version ${k8s}, expected k8s=X.Y"
        fi
        vm-command "snap $snapcmd $k8sverparam $kubepart --classic"
    done
    # Manage kubelet with systemd rather than snap
    vm-command "snap stop kubelet"
cat <<EOF |
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/snap/bin/kubelet --bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf --config=/var/lib/kubelet/config.yaml --container-runtime=remote --container-runtime-endpoint=${k8scri_sock} --pod-infra-container-image=k8s.gcr.io/pause:3.4.1
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    vm-pipe-to-file /etc/systemd/system/kubelet.service
    vm-command "systemctl enable --now kubelet" ||
        command-error "failed to enable kubelet"
}

opensuse-install-kernel-dev() {
    vm-command-q "zypper lr | grep -q openSUSE_Tools" ||
        distro-install-repo "http://download.opensuse.org/repositories/openSUSE:/Tools/openSUSE_Factory/openSUSE:Tools.repo"
    distro-install-pkg "git-core make gcc flex bison bc ncurses-devel patch bzip2 osc build python quilt"
    vm-command "cd /root; [ -d kernel ] || git clone --depth=100 https://github.com/SUSE/kernel"
    vm-command "cd /root; [ -d kernel-source ] || git clone --depth=100 https://github.com/SUSE/kernel-source"
    vm-command "[ -f /etc/profile.d/linux_git.sh ] || echo export LINUX_GIT=/root/kernel > /etc/profile.d/linux_git.sh"
}

opensuse-bootstrap-commands-pre() {
    cat <<EOF
sed -e '/Signature checking/a gpgcheck = off' -i /etc/zypp/zypp.conf
EOF
}

opensuse-govm-env() {
    echo "DISABLE_VGA=N"
}

###########################################################################

#
# Generic rpm functions
#

rpm-pkg-type() {
    echo rpm
}

rpm-install-repo-key() {
    local key
    for key in "$@"; do
        vm-command "rpm --import $key" ||
            command-error "failed to import repo key $key"
    done
}

rpm-refresh-pkg-db() {
    :
}

###########################################################################

#
# default implementations
#

default-bootstrap-commands() {
    cat <<EOF
rm -f /etc/modules-load.d/k8s.conf; touch /etc/modules-load.d/k8s.conf
modprobe bridge && echo bridge >> /etc/modules-load.d/k8s.conf || :
modprobe nf-tables-bridge && echo nf-tables-bridge >> /etc/modules-load.d/k8s.conf || :
modprobe br_netfilter && echo br_netfilter >> /etc/modules-load.d/k8s.conf || :

touch /etc/sysctl.d/k8s.conf
echo "net.bridge.bridge-nf-call-ip6tables = 1" >> /etc/sysctl.d/k8s.conf
echo "net.bridge.bridge-nf-call-iptables = 1" >> /etc/sysctl.d/k8s.conf
echo "net.ipv4.ip_forward = 1" >> /etc/sysctl.d/k8s.conf

# rp_filter (partially) mitigates DDOS attacks with spoofed IP addresses
# by dropping packages with non-routable (unanswerable) source addresses.
# However, rp_filter > 0 breaks cilium networking. Make sure it's disabled.
echo "net.ipv4.conf.*.rp_filter = 0" >> /etc/sysctl.d/k8s.conf

/sbin/sysctl -p /etc/sysctl.d/k8s.conf || :
EOF
}

default-setup-proxies() {
    # Notes:
    #   We blindly assume that upper- vs. lower-case env vars are identical.
    # shellcheck disable=SC2154
    if [ -z "$http_proxy$https_proxy$ftp_proxy$no_proxy" ]; then
        return 0
    fi
    if vm-command-q "grep -q \"http_proxy=$http_proxy\" /etc/profile.d/proxy.sh && \
                     grep -q \"https_proxy=$https_proxy\" /etc/profile.d/proxy.sh && \
                     grep -q \"ftp_proxy=$ftp_proxy\" /etc/profile.d/proxy.sh && \
                     grep -q \"no_proxy=$no_proxy\" /etc/profile.d/proxy.sh" 2>/dev/null; then
        # No changes in proxy configuration
        return 0
    fi

    local file scope="" append="--append" hn ext_no_proxy
    hn="$(vm-command-q hostname)"

    local master_node_ip_comma=""
    if [ -n "$k8smaster" ]; then
        local master_user_ip
        master_user_ip="$(vm-ssh-user-ip $k8smaster)"
        master_node_ip_comma=${master_user_ip/*@},
    fi

    ext_no_proxy="$master_node_ip_comma$VM_IP,10.0.0.0/8,$CNI_SUBNET,$hn,.svc,.internal,192.168.0.0/16"

    for file in /etc/environment /etc/profile.d/proxy.sh; do
        cat <<EOF |
${scope}http_proxy=$http_proxy
${scope}https_proxy=$https_proxy
${scope}ftp_proxy=$ftp_proxy
${scope}no_proxy=$no_proxy,$ext_no_proxy
${scope}HTTP_PROXY=$http_proxy
${scope}HTTPS_PROXY=$https_proxy
${scope}FTP_PROXY=$ftp_proxy
${scope}NO_PROXY=$no_proxy,$ext_no_proxy
EOF
      vm-pipe-to-file $append $file
      scope="export "
      append=""
    done
    # Setup proxies for systemd services that might be installed later
    for file in /etc/systemd/system/{containerd,docker,crio}.service.d/proxy.conf; do
        cat <<EOF |
[Service]
Environment=HTTP_PROXY=$http_proxy
Environment=HTTPS_PROXY=$https_proxy
Environment=NO_PROXY=$no_proxy,$ext_no_proxy
EOF
        vm-pipe-to-file $file
    done
    # Setup proxies inside docker containers
    for file in /{root,home/$VM_SSH_USER}/.docker/config.json; do
        cat <<EOF |
{
    "proxies": {
        "default": {
            "httpProxy": "$http_proxy",
            "httpsProxy": "$https_proxy",
            "noProxy": "$no_proxy,$ext_no_proxy"
        }
    }
}
EOF
        vm-pipe-to-file $file
    done
}

default-setup-oneshot() {
    :
}

default-install-utils() {
    # $distro-install-utils() is responsible for installing common
    # utilities, such as pidof and killall, that the test framework
    # and tests in general can expect to be found on VM.
    :
}

default-k8s-cni() {
    echo ${k8scni:-cilium}
}

default-k8s-cni-subnet() {
    if [ "$(distro-k8s-cni)" == "flannel" ]; then
        echo 10.244.0.0/16
    else
        echo 10.217.0.0/16
    fi
}

default-install-runc() {
    distro-install-pkg runc
}

default-install-containerd() {
    vm-command-q "[ -f /usr/bin/containerd ]" || {
        distro-install-pkg containerd
    }
}

default-config-containerd() {
    if vm-command-q "[ ! -f /etc/containerd/config.toml ]"; then
        vm-command "mkdir -p /etc/containerd && containerd config default > /etc/containerd/config.toml"
    fi

    vm-sed-file /etc/containerd/config.toml 's/^.*disabled_plugins *= *.*$/disabled_plugins = []/'

    if vm-command-q "containerd config dump | grep -v -q SystemdCgroup"; then
        vm-command "containerd config dump > /etc/containerd/config.toml"
    fi

    vm-sed-file /etc/containerd/config.toml 's/SystemdCgroup = false/SystemdCgroup = true/g'
}

default-restart-containerd() {
    vm-command "systemctl daemon-reload && systemctl restart containerd" ||
        command-error "failed to restart containerd systemd service"
}

default-install-crio() {
    [ -n "$crio_src" ] || error "crio install error: crio_src is not set"
    [ -x "$crio_src/bin/crio" ] || error "crio install error: file not found $crio_src/bin/crio"
    for f in crio crio-status pinns; do
        vm-put-file "$crio_src/bin/$f" "/usr/bin/$f"
    done
    cat <<EOF |
[Unit]
Description=cri-o container runtime
Documentation=https://cri-o.io
After=network.target

[Service]
ExecStart=/usr/bin/crio

Delegate=yes
KillMode=process
Restart=always
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=1048576
TasksMax=infinity

[Install]
WantedBy=multi-user.target
EOF
    vm-pipe-to-file /etc/systemd/system/crio.service
    vm-command "mkdir -p /etc/systemd/system/crio.service.d"
    vm-command "(echo \"[Service]\"; echo \"Environment=PATH=/sbin:/usr/sbin:$PATH:/usr/libexec/podman\") > /etc/systemd/system/crio.service.d/path.conf; systemctl daemon-reload"
}

default-config-crio() {
    vm-command "mkdir -p /etc/containers"
    echo '{"default": [{"type":"insecureAcceptAnything"}]}' | vm-pipe-to-file /etc/containers/policy.json
    cat <<EOF |
[registries.search]
registries = ['docker.io']
EOF
    vm-pipe-to-file /etc/containers/registries.conf
}

default-restart-crio() {
    vm-command "systemctl daemon-reload && systemctl restart crio" ||
        command-error "failed to restart crio systemd service"
}

default-install-minikube() {
    vm-command "curl -Lo /usr/local/bin/minikube https://storage.googleapis.com/minikube/releases/${MINIKUBE_VERSION}/minikube-linux-amd64 && chmod +x /usr/local/bin/minikube"
    distro-install-crictl
}

default-install-crictl() {
    vm-command "set -e -x
    wget https://github.com/kubernetes-sigs/cri-tools/releases/download/${CRICTL_VERSION}/crictl-${CRICTL_VERSION}-linux-amd64.tar.gz
    sudo tar zxvf crictl-${CRICTL_VERSION}-linux-amd64.tar.gz -C /usr/local/bin
    rm -f crictl-${CRICTL_VERSION}-linux-amd64.tar.gz
    "
}

default-install-cri-dockerd() {
    vm-command "set -e -x
    git clone --depth=1 https://github.com/Mirantis/cri-dockerd.git
    cd cri-dockerd
    mkdir bin
    go build -o bin/cri-dockerd
    mkdir -p /usr/local/bin
    install -o root -g root -m 0755 bin/cri-dockerd /usr/local/bin/cri-dockerd
    cp -a packaging/systemd/* /etc/systemd/system
    sed -i -e 's,/usr/bin/cri-dockerd,/usr/local/bin/cri-dockerd,' /etc/systemd/system/cri-docker.service
    systemctl daemon-reload
    systemctl enable cri-docker.service
    systemctl enable --now cri-docker.socket
    "
}

default-govm-env() {
    echo "DISABLE_VGA=Y"
}

default-env-file-dir() {
    echo "/etc/sysconfig"
}

###########################################################################

#
# generic supporting functions
#

from-tarball-install-golang() {
    vm-command-q "go version | grep -q go$GOLANG_VERSION" || {
        vm-command "wget --progress=dot:giga $GOLANG_URL -O go.tgz" && \
            vm-command "tar -C /usr/local -xvzf go.tgz >/dev/null && rm go.tgz" && \
            vm-command "echo 'PATH=/usr/local/go/bin:\$PATH' > /etc/profile.d/go.sh" && \
            vm-command "echo \* installed \$(go version)"
    }
}

create-ext4-var-lib-containerd() {
    local dir="/var/lib/containerd" file="/loop-ext4.dsk" dev

    echo "Creating loopback-mounted ext4 $dir..."

    if ! dev="$(vm-command-q "losetup -f")" || [ -z "$dev" ]; then
        command-error "failed to find unused loopback device"
    fi
    vm-command "dd if=/dev/zero of=$file bs=$((1024*1000)) count=$((1000*5))" ||
        command-error "failed to create file for ext4 loopback mount"
    vm-command "losetup $dev $file" ||
        command-error "failed to attach $file to $dev"
    vm-command "mkfs.ext4 $dev" ||
        command-error "failed to create ext4 filesystem on $dev ($file)"
    if vm-command "[ -d $dir ]"; then
        vm-command "mv $dir $dir.orig" ||
            command-error "failed to rename original $dir to $dir.orig"
    fi
    vm-command "mkdir -p $dir" ||
        command-error "failed to create $dir"

    cat <<EOF |
[Unit]
Description=Activate loop device
DefaultDependencies=no
After=systemd-udev-settle.service
Wants=systemd-udev-settle.service

[Service]
ExecStart=/sbin/losetup $dev $file
Type=oneshot

[Install]
WantedBy=local-fs.target
EOF
    vm-pipe-to-file /etc/systemd/system/attach-loop-devices.service
    vm-command "systemctl enable attach-loop-devices.service"

    cat <<EOF |
$dev    $dir    ext4    defaults    1 2
EOF
    vm-pipe-to-file --append /etc/fstab

    vm-command "mount $dir" ||
        command-error "failed to mount new ext4 $dir"
    if vm-command "[ -d $dir.orig ]"; then
        vm-command "tar -C $dir.orig -cf - . | tar -C $dir -xf -" ||
            command-error "failed to copy $dir.orig to $dir"
    fi
}

CNI_SUBNET=$(distro-k8s-cni-subnet)
