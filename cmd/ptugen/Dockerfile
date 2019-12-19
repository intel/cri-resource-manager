FROM alpine as builder

RUN apk update && apk add bash && rm -fr /var/cache/apk/*
COPY cmd/ptugen/ptugen /bin/ptugen
