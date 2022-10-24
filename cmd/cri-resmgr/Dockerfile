ARG GO_VERSION=1.19

FROM golang:${GO_VERSION}-bullseye as builder

WORKDIR /go/build

# Fetch go dependencies in a separate layer for caching
COPY go.mod go.sum ./
COPY pkg/topology/ pkg/topology/
RUN go mod download

# Build cri-resmgr, fully statically linked binary
COPY . .

RUN make build-static BUILD_DIRS="cri-resmgr"

FROM scratch as final

COPY --from=builder /go/build/bin/cri-resmgr /bin/cri-resmgr
COPY --from=builder /go/build/cmd/cri-resmgr/fallback.cfg.sample /etc/cri-resmgr/fallback.cfg

ENTRYPOINT ["/bin/cri-resmgr"]
