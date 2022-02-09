### builder
FROM golang:1.17.6-alpine AS builder

WORKDIR /go/src/github.com/intel/cri-resource-manager/packaging/gardener
COPY cmd .
COPY go.mod .
COPY go.sum .
RUN go install ./...

### extension
FROM alpine:3.15.0 AS gardener-extension-cri-rm

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-cri-rm /gardener-extension-cri-rm
ENTRYPOINT ["/gardener-extension-cri-rm"]

### installation
FROM ubuntu:22.04 AS gardener-extension-cri-rm-installation
RUN apt update -y && apt install -y make wget
COPY Makefile .
RUN make install-binaries

