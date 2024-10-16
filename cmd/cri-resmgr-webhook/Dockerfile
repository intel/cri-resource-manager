ARG GO_VERSION=1.22

FROM golang:${GO_VERSION}-bullseye as builder

ARG GOLICENSES_VERSION

WORKDIR /go/build

# Fetch go dependencies in a separate layer for caching
RUN go install github.com/google/go-licenses@${GOLICENSES_VERSION}
COPY go.mod go.sum ./
COPY pkg/topology/ pkg/topology/
RUN go mod download -x

# Build webhook, fully statically linked binary
COPY . .

RUN CGO_ENABLED=0 make build-static BUILD_DIRS=cri-resmgr-webhook && \
    install -D /go/build/bin/* -t /install_root/bin

# Save licenses
RUN make install-licenses BUILD_DIRS=cri-resmgr-webhook DESTDIR=/install_root

FROM scratch as final

USER 65534:65534

COPY --from=builder /install_root /

ENTRYPOINT ["/bin/cri-resmgr-webhook"]

