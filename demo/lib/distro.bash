GO_URLDIR=https://golang.org/dl
GO_VERSION=1.14.9
GOLANG_URL=$GO_URLDIR/go$GO_VERSION.linux-amd64.tar.gz

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
distro-install-crio()       { distro-resolve "$@"; }
distro-install-k8s()        { distro-resolve "$@"; }
distro-set-kernel-cmdline() { distro-resolve "$@"; }
distro-bootstrap-commands() { distro-resolve "$@"; }

# default no-op fallbacks for optional API functions
default-bootstrap-commands() { :; }

###########################################################################

# distro-specific function resolution
distro-resolve() {
    # We dig out the distro-* API function name from stack of callers,
    # then try resolving it to an implementation. The resolution
    # process goes through a list of potential implementation names in
    # decreasing order of distro/version-specificity, then tries a few
    # fallbacks based on known distro relations and properties.
    local apifn="${FUNCNAME[1]}" fallbacks fn
    case $apifn in
        distro-*) apifn="${apifn#distro-}";;
        *) error "internal error: ${FUNCNAME[0]} called by non-API $apifn";;
    esac
    case $VM_DISTRO in
        ubuntu*) fallbacks="debian-$apifn";;
        centos*) fallbacks="fedora-$apifn rpm-$apifn";;
        fedora*) fallbacks="rpm-$apifn";;
        *suse*)  fallbacks="rpm-$apifn";;
    esac
    fallbacks="$fallbacks default-$apifn distro-unresolved"
    # try version-based resolution first, then derivative fallbacks
    for fn in "${VM_DISTRO/./_}-$apifn" "${VM_DISTRO%-*}-$apifn" $fallbacks; do
        if [ "$(type -t -- "$fn")" = "function" ]; then
            $fn "$@"
            return $?
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
    vm-command "command -v gpg >/dev/null 2>&1" || debian-install-pkg gnupg2
    for key in "$@"; do
        vm-command "curl -s $key | apt-key add -" ||
            command-error "failed to install repo key $key"
    done
}

debian-install-repo() {
    vm-command-q "type -t add-apt-repository >& /dev/null" || {
        vm-command "apt-get update && apt-get install -y software-properties-common" ||
            command-error "failed to install software-properties-common"
    }
    vm-command "add-apt-repository \"$*\"" ||
        command-error "failed to install apt repository $*"
    debian-refresh-pkg-db
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

debian-10-install-containerd() {
    vm-command-q "[ -f /usr/bin/containerd ]" || {
        debian-refresh-pkg-db
        debian-install-repo-key https://download.docker.com/linux/debian/gpg
        debian-install-repo "deb https://download.docker.com/linux/debian buster stable"
        debian-refresh-pkg-db
        debian-install-pkg containerd
        generic-setup-containerd
    }
}

debian-install-containerd() {
    vm-command-q "[ -f /usr/bin/containerd ]" || {
        debian-refresh-pkg-db
        debian-install-pkg containerd
        # The default Debian containerd expects CNI binaries in /usr/lib/cni,
        # but kubernetes-cni.deb (debian-install-k8s) installs to /opt/cni/bin.
        vm-command "sed -e 's|bin_dir = \"/usr/lib/cni\"|bin_dir = \"/opt/cni/bin\"|g' -i /etc/containerd/config.toml"
        generic-setup-containerd
    }
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
    echo "https://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud-1503.qcow2.xz"
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

centos-install-golang() {
    distro-install-pkg wget tar gzip
    from-tarball-install-golang
}

centos-install-containerd() {
    vm-command-q "[ -f /usr/bin/containerd ]" || {
        distro-install-repo https://download.docker.com/linux/centos/docker-ce.repo
        distro-install-pkg containerd
    }
    generic-setup-containerd
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

fedora-install-containerd() {
    vm-command-q "[ -f /usr/bin/containerd ]" || {
        distro-install-repo https://download.docker.com/linux/fedora/docker-ce.repo
        distro-install-pkg containerd
    }
    generic-setup-containerd
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

    vm-command "setenforce 0" ||
        command-error "failed to runtime-disable selinux"
    vm-sed-file /etc/selinux/config 's/^SELINUX=.*$/SELINUX=permissive/'
    distro-install-pkg tc kubelet kubeadm kubectl
    vm-command "systemctl enable --now kubelet" ||
        command-error "failed to enable kubelet"
}

fedora-bootstrap-commands() {
    cat <<EOF
mkdir -p /etc/sudoers.d
echo 'Defaults !requiretty' > /etc/sudoers.d/10-norequiretty
setenforce 0
sed -E -i 's/^SELINUX=.*$/SELINUX=permissive/' /etc/selinux/config
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
${scope}no_proxy=$no_proxy,$VM_IP,10.96.0.0/12,10.217.0.0/16,$hn,.svc
${scope}HTTP_PROXY=$http_proxy
${scope}HTTPS_PROXY=$https_proxy
${scope}FTP_PROXY=$ftp_proxy
${scope}NO_PROXY=$no_proxy,$VM_IP,10.96.0.0/12,10.217.0.0/16,$hn,.svc
EOF
      vm-pipe-to-file $append $file
      scope="export "
      append=""
    done
    # Setup proxies for systemd services that might be installed later
    for file in /etc/systemd/system/{containerd,docker}.service.d/proxy.conf; do
        cat <<EOF |
[Service]
Environment=HTTP_PROXY="$http_proxy"
Environment=HTTPS_PROXY="$https_proxy"
Environment=NO_PROXY="$no_proxy,$VM_IP,10.96.0.0/12,10.217.0.0/16,$hn,.svc"
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

generic-setup-containerd() {
    if vm-command-q "[ -f /etc/containerd/config.toml ]"; then
        vm-sed-file /etc/containerd/config.toml 's/^.*disabled_plugins *= *.*$/disabled_plugins = []/'
    fi
    vm-command "systemctl daemon-reload && systemctl restart containerd" ||
        command-error "failed to restart containerd systemd service"
}
