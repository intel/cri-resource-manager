# Build webhook
FROM golang:alpine as builder

RUN apk add --no-cache gcc git

ADD . /go/src/build

WORKDIR /go/src/build

ENV GO111MODULE=on
RUN go build -o cmd/webhook/webhook ./cmd/webhook/*go

# Create production image without the build deps
FROM alpine

RUN apk add --no-cache libc6-compat

COPY --from=builder /go/src/build/cmd/webhook/webhook /bin/webhook
COPY --from=builder /go/src/build/test/data/cri-resmgr-webhook.cri-resmgr.svc.* /etc/cri-resmgr-webhook/certs.d/

ENTRYPOINT ["/bin/webhook", "-cert-file=/etc/cri-resmgr-webhook/certs.d/cri-resmgr-webhook.cri-resmgr.svc.crt", "-key-file=/etc/cri-resmgr-webhook/certs.d/cri-resmgr-webhook.cri-resmgr.svc.key"]
