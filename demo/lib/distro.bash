# shellcheck disable=SC2120
GO_URLDIR=https://golang.org/dl
GO_VERSION=1.14.9
GOLANG_URL=$GO_URLDIR/go$GO_VERSION.linux-amd64.tar.gz
CNI_SUBNET=10.217.0.0/16

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
distro-remove-pkg()         { distro-resolve "$@"; }
distro-setup-proxies()      { distro-resolve "$@"; }
distro-install-golang()     { distro-resolve "$@"; }
distro-install-containerd() { distro-resolve "$@"; }
distro-config-containerd()  { distro-resolve "$@"; }
distro-restart-containerd() { distro-resolve "$@"; }
distro-install-crio()       { distro-resolve "$@"; }
distro-config-crio()        { distro-resolve "$@"; }
distro-restart-crio()       { distro-resolve "$@"; }
distro-install-k8s()        { distro-resolve "$@"; }
distro-k8s-cni()            { distro-resolve "$@"; }
distro-set-kernel-cmdline() { distro-resolve "$@"; }
distro-bootstrap-commands() { distro-resolve "$@"; }

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

ubuntu-20_10-image-url() {
    echo "https://cloud-images.ubuntu.com/groovy/current/groovy-server-cloudimg-amd64.img"
}

debian-10-image-url() {
    echo "https://cloud.debian.org/images/cloud/buster/20200803-347/debian-10-generic-amd64-20200803-347.qcow2"
}

debian-sid-image-url() {
    echo "https://cloud.debian.org/images/cloud/sid/daily/20201013-422/debian-sid-generic-amd64-daily-20201013-422.qcow2"
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
        vm-command "curl -s $key | apt-key add -" ||
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
    vm-command "apt-get install -y $*" ||
        command-error "failed to install $*"
}

debian-remove-pkg() {
    vm-command "for pkg in $*; do dpkg -l \$pkg >& /dev/null && apt remove -y --purge \$pkg || :; done" ||
        command-error "failed to remove package(s) $*"
}

debian-install-golang() {
    debian-install-pkg golang
}

debian-10-install-containerd-pre() {
    debian-install-repo-key https://download.docker.com/linux/debian/gpg
    debian-install-repo "deb https://download.docker.com/linux/debian buster stable"

}

debian-sid-install-containerd-post() {
    vm-command "sed -e 's|bin_dir = \"/usr/lib/cni\"|bin_dir = \"/opt/cni/bin\"|g' -i /etc/containerd/config.toml"
}

debian-install-k8s() {
    debian-refresh-pkg-db
    debian-install-pkg apt-transport-https curl
    debian-install-repo-key "https://packages.cloud.google.com/apt/doc/apt-key.gpg"
    debian-install-repo "deb https://apt.kubernetes.io/ kubernetes-xenial main"
    debian-install-pkg kubeadm kubelet kubectl
    vm-command "apt-get update && apt-get install -y kubelet kubeadm kubectl" ||
        command-error "failed to install kubernetes packages"
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


###########################################################################

#
# Centos 7, 8, generic Fedora, generic rpm functions
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

centos-8-install-containerd-pre() {
    distro-install-repo https://download.docker.com/linux/centos/docker-ce.repo
}

centos-7-install-k8s-post() {
    vm-sed-file /etc/sysconfig/kubelet 's/^KUBELET_EXTRA_ARGS=/KUBELET_EXTRA_ARGS="--feature-gates=SupportNodePidsLimit=false,SupportPodPidsLimit=false"/'
}

centos-7-k8s-cni() {
    echo "weavenet"
}

centos-install-golang() {
    distro-install-pkg wget tar gzip
    from-tarball-install-golang
}

fedora-image-url() {
    echo "https://download.fedoraproject.org/pub/fedora/linux/releases/32/Cloud/x86_64/images/Fedora-Cloud-Base-32-1.6.x86_64.qcow2"
}

fedora-ssh-user() {
    echo fedora
}

fedora-install-repo() {
    distro-install-pkg dnf-plugins-core
    vm-command "dnf config-manager --add-repo $*" ||
        command-error "failed to install DNF repository $*"
}

fedora-install-pkg() {
    vm-command "dnf install -y $*" ||
        command-error "failed to install $*"
}

fedora-remove-pkg() {
    vm-command "dnf remove -y $*" ||
        command-error "failed to remove package(s) $*"
}

fedora-install-golang() {
    fedora-install-pkg wget tar gzip
    from-tarball-install-golang
}

fedora-install-containerd-pre() {
    distro-install-repo https://download.docker.com/linux/fedora/docker-ce.repo
}

fedora-install-containerd-post() {
    distro-install-pkg containernetworking-plugins
}

fedora-config-containerd-post() {
    if [ "$VM_DISTRO" = "fedora" ]; then
        vm-command "mkdir -p /opt/cni/bin && mount --bind /usr/libexec/cni /opt/cni/bin"
        vm-command "echo /usr/libexec/cni /opt/cni/bin none defaults,bind,nofail 0 0 >> /etc/fstab"
    fi
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

    distro-install-pkg tc kubelet kubeadm kubectl
    vm-command "systemctl enable --now kubelet" ||
        command-error "failed to enable kubelet"
}

fedora-bootstrap-commands-pre() {
    cat <<EOF
mkdir -p /etc/sudoers.d
echo 'Defaults !requiretty' > /etc/sudoers.d/10-norequiretty

setenforce 0
sed -E -i 's/^SELINUX=.*$/SELINUX=permissive/' /etc/selinux/config

if grep -q NAME=Fedora /etc/os-release; then
    if ! grep -q systemd.unified_cgroup_hierarchy=0 /proc/cmdline; then
        sed -i -E 's/^kernelopts=(.*)/kernelopts=\1 systemd.unified_cgroup_hierarchy=0/' /boot/grub2/grubenv
        shutdown -r now
    fi
fi

echo PATH="\$PATH:/usr/local/bin:/usr/local/sbin" > /etc/profile.d/usr-local-path.sh
EOF
}

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
touch /etc/modules-load.d/k8s.conf
modprobe bridge && echo bridge >> /etc/modules-load.d/k8s.conf || :
modprobe nf-tables-bridge && echo nf-tables-bridge >> /etc/modules-load.d/k8s.conf || :
modprobe br_netfilter && echo br_netfilter >> /etc/modules-load.d/k8s.conf || :

touch /etc/sysctl.d/k8s.conf
echo "net.bridge.bridge-nf-call-ip6tables = 1" >> /etc/sysctl.d/k8s.conf
echo "net.bridge.bridge-nf-call-iptables = 1" >> /etc/sysctl.d/k8s.conf
echo "net.ipv4.ip_forward = 1" >> /etc/sysctl.d/k8s.conf
/sbin/sysctl -p /etc/sysctl.d/k8s.conf
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

    local file scope="" append="--append" hn
    hn="$(vm-command-q hostname)"

    for file in /etc/environment /etc/profile.d/proxy.sh; do
        cat <<EOF |
${scope}http_proxy=$http_proxy
${scope}https_proxy=$https_proxy
${scope}ftp_proxy=$ftp_proxy
${scope}no_proxy=$no_proxy,$VM_IP,10.96.0.0/12,$CNI_SUBNET,$hn,.svc
${scope}HTTP_PROXY=$http_proxy
${scope}HTTPS_PROXY=$https_proxy
${scope}FTP_PROXY=$ftp_proxy
${scope}NO_PROXY=$no_proxy,$VM_IP,10.96.0.0/12,$CNI_SUBNET,$hn,.svc
EOF
      vm-pipe-to-file $append $file
      scope="export "
      append=""
    done
    # Setup proxies for systemd services that might be installed later
    for file in /etc/systemd/system/{containerd,docker}.service.d/proxy.conf; do
        cat <<EOF |
[Service]
Environment=HTTP_PROXY=$http_proxy
Environment=HTTPS_PROXY=$https_proxy
Environment=NO_PROXY=$no_proxy,$VM_IP,10.96.0.0/12,$CNI_SUBNET,$hn,.svc
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
            "noProxy": "$no_proxy,$VM_IP,$hn"
        }
    }
}
EOF
        vm-pipe-to-file $file
    done
}

default-k8s-cni() {
    echo cilium
}

default-install-containerd() {
    vm-command-q "[ -f /usr/bin/containerd ]" || {
        distro-refresh-pkg-db
        distro-install-pkg containerd
    }
}

default-config-containerd() {
    if vm-command-q "[ -f /etc/containerd/config.toml ]"; then
        vm-sed-file /etc/containerd/config.toml 's/^.*disabled_plugins *= *.*$/disabled_plugins = []/'
    fi
}

default-restart-containerd() {
    vm-command "systemctl daemon-reload && systemctl restart containerd" ||
        command-error "failed to restart containerd systemd service"
}

###########################################################################

#
# generic supporting functions
#

from-tarball-install-golang() {
    vm-command-q "go version | grep -q go$GOLANG_VERSION" || {
        vm-command "wget --progress=dot:giga $GOLANG_URL -O go.tgz" && \
            vm-command "tar -C /usr/local -xvzf go.tgz && rm go.tgz" && \
            vm-command "echo \"PATH=/usr/local/go/bin:\$PATH\" > /etc/profile.d/go.sh" && \
            vm-command "* installed \$(go version)"
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
