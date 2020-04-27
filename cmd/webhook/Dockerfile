# CLEAR_LINUX_BASE and CLEAR_LINUX_VERSION can be used to make the build
# reproducible by choosing an image by its hash and installing an OS version
# with --version=:
# CLEAR_LINUX_BASE=clearlinux@sha256:b8e5d3b2576eb6d868f8d52e401f678c873264d349e469637f98ee2adf7b33d4
# CLEAR_LINUX_VERSION="--version=29970"
#
# This is used on release branches before tagging a stable version.
# The master branch defaults to using the latest Clear Linux.
ARG CLEAR_LINUX_BASE=clearlinux/golang:latest

FROM ${CLEAR_LINUX_BASE} as builder

ARG CLEAR_LINUX_VERSION=

RUN swupd update --no-boot-update ${CLEAR_LINUX_VERSION}
RUN swupd bundle-add make ${CLEAR_LINUX_VERSION}
RUN mkdir /install_root \
    && swupd os-install \
    ${CLEAR_LINUX_VERSION} \
    --path /install_root \
    --statedir /swupd-state \
    --bundles=os-core \
    --no-boot-update \
    && rm -rf /install_root/var/lib/swupd/*

WORKDIR /go/build

# Fetch go dependencies in a separate layer for caching
COPY go.mod go.sum ./
COPY pkg/topology/ pkg/topology/
RUN go mod download

# Build webhook
COPY . .
RUN make BUILD_DIRS=webhook

FROM scratch as final
ARG DIR=/go/src/build
COPY --from=builder /install_root /

COPY --from=builder /go/build/bin/webhook /bin/webhook
COPY --from=builder /go/build/test/data/cri-resmgr-webhook.cri-resmgr.svc.* /etc/cri-resmgr-webhook/certs.d/

ENTRYPOINT ["/bin/webhook", "-cert-file=/etc/cri-resmgr-webhook/certs.d/cri-resmgr-webhook.cri-resmgr.svc.crt", "-key-file=/etc/cri-resmgr-webhook/certs.d/cri-resmgr-webhook.cri-resmgr.svc.key"]

