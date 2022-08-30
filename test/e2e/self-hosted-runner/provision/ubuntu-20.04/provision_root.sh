#!/usr/bin/env bash

if [ ! -z "$DNS_NAMESERVER" ]; then
    # Make sure the resolv.conf is not sym linked to stub-resolv.conf
    rm -f /etc/resolv.conf
    touch /etc/resolv.conf

    # Setup search domain and nameserver
    if [ ! -z "$DNS_SEARCH_DOMAIN" ]; then
	echo "search $DNS_SEARCH_DOMAIN" >> /etc/resolv.conf
    fi

    echo "nameserver $DNS_NAMESERVER" >> /etc/resolv.conf
fi

if [ ! -z "$REPO_PATH_CACHED_PACKAGES" ]; then
    # Use our cache of ubuntu packages
    cat > /etc/apt/sources.list <<EOF
deb $REPO_PATH_CACHED_PACKAGES focal main restricted
deb $REPO_PATH_CACHED_PACKAGES focal-updates main restricted
deb $REPO_PATH_CACHED_PACKAGES focal universe
deb $REPO_PATH_CACHED_PACKAGES focal-updates universe
deb $REPO_PATH_CACHED_PACKAGES focal multiverse
deb $REPO_PATH_CACHED_PACKAGES focal-updates multiverse
deb $REPO_PATH_CACHED_PACKAGES focal-backports main restricted universe multiverse
deb http://security.ubuntu.com/ubuntu focal-security main restricted
deb http://security.ubuntu.com/ubuntu focal-security universe
deb http://security.ubuntu.com/ubuntu focal-security multiverse
EOF
fi

mkdir -p /etc/systemd/system/{containerd,docker,crio}.service.d
for file in /etc/systemd/system/{containerd,docker,crio}.service.d/proxy.conf; do
  cat > $file <<EOF
[Service]
Environment=HTTP_PROXY=$PROXY_HTTP
Environment=HTTPS_PROXY=PROXY_HTTPS
Environment=NO_PROXY=$PROXY_NO,10.96.0.0/12,$.svc
EOF
done

# Setup proxies inside docker containers
mkdir -p /home/vagrant/.docker
for file in /home/vagrant/.docker/config.json; do
  cat > $file <<EOF
{
    "proxies": {
        "default": {
            "httpProxy": "$PROXY_HTTP",
            "httpsProxy": "$PROXY_HTTPS",
            "noProxy": "$PROXY_NO"
        }
    }
}
EOF
done

# fail on unset variables and command errors
set -eu -o pipefail # -x: is for debugging

apt-get update
apt-get upgrade -y
apt-get install -y software-properties-common
#add-apt-repository --yes --update ppa:git-core/ppa
apt-get install -y \
  bash \
  build-essential \
  clang-format \
  git \
  git-lfs \
  jq \
  pv \
  xz-utils \
  bzip2 \
  gzip \
  libffi-dev \
  libssl-dev \
  python3 \
  python3-dev \
  python3-pip \
  python3-venv \
  shellcheck \
  tree \
  wget \
  yamllint \
  zstd

# Install docker
apt-get install -y apt-transport-https ca-certificates curl gnupg lsb-release
if [ ! -f /usr/share/keyrings/docker-archive-keyring.gpg ]; then
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
fi
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
apt-get update -y
apt-get install -y docker-ce docker-ce-cli containerd.io
groupadd docker || true
gpasswd -a vagrant docker
newgrp docker
systemctl restart docker

# golang
add-apt-repository --yes --update ppa:longsleep/golang-backports
apt-get install -y golang

# goreleaser
echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
apt-get update
apt-get install -y goreleaser

# Cleanup
apt-get autoremove -y
