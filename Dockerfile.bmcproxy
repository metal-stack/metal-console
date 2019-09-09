FROM registry.fi-ts.io/cloud-native/go-builder:latest as builder

FROM alpine:3.10

LABEL maintainer="FI-TS Devops <devops@f-i-ts.de>"

RUN apk add \
        ipmitool \
        libvirt-client

COPY host-key /
COPY --from=builder /work/bin/bmc-proxy /

CMD ["/bmc-proxy"]
