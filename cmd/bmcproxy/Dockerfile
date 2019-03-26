FROM golang:1.11-stretch as builder
ARG SHA
ARG GITVERSION
RUN apt update \
 && apt -y install \
    make
WORKDIR /app
ENV GOPROXY=https://gomods.fi-ts.io

# Install dependencies
COPY go.mod .
RUN go mod download

# Build
COPY metal-api.json Makefile ./
RUN make BIN_DIR=bin swagger
COPY internal ./internal
COPY main.go ./
RUN make BIN_DIR=bin SHA=${SHA} GITVERSION=${GITVERSION} all

FROM alpine:3.9
LABEL maintainer FI-TS Devops <devops@f-i-ts.de>
COPY --from=builder /app/bin/bmc-proxy /bmc-proxy
RUN apk add \
        ipmitool \
        libvirt-client
COPY id_rsa /id_rsa
CMD ["/bmc-proxy"]
