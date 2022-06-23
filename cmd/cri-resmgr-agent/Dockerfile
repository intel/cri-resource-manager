ARG GO_VERSION=1.18

FROM golang:${GO_VERSION}-bullseye as builder

WORKDIR /go/build

# Fetch go dependencies in a separate layer for caching
COPY go.mod go.sum ./
COPY pkg/topology/ pkg/topology/
RUN go mod download

# Build webhook, fully statically linked binary
COPY . .

RUN CGO_ENABLED=0 make BUILD_DIRS="cri-resmgr-agent cri-resmgr-agent-probe"

FROM scratch as final

COPY --from=builder /go/build/bin/* /bin/

ENTRYPOINT ["/bin/cri-resmgr-agent"]
