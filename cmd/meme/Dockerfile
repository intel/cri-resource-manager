ARG GO_VERSION=1.19

FROM golang:${GO_VERSION}-bullseye as builder
WORKDIR /go/build
COPY . .
RUN make STATIC=1 bin/meme && \
    install -D /go/build/bin/meme -t /install_root/bin

FROM scratch as final
COPY --from=builder /install_root /
ENTRYPOINT ["/bin/meme"]
