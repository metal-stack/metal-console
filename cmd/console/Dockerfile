FROM registry.fi-ts.io/cloud-native/go-builder:latest as builder

FROM alpine:3.9
LABEL maintainer FI-TS Devops <devops@f-i-ts.de>
COPY id_rsa ca.crt client.crt client.pem /
COPY --from=builder /work/bin/metal-console /
CMD ["/metal-console"]
