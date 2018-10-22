FROM golang:1.11-stretch as builder
RUN apt update \
 && apt -y install make git
WORKDIR /app
ENV GOPROXY=https://gomods.fi-ts.io

# Install dependencies
COPY go.mod .
RUN go mod download

# Build
COPY .git ./.git
COPY cmd ./cmd
COPY main.go ./main.go
COPY metal-api.json ./metal-api.json
COPY Makefile ./Makefile
RUN make swagger \
 && make

FROM alpine:3.8
LABEL maintainer FI-TS Devops <devops@f-i-ts.de>
COPY --from=builder /app/bin/metal-console /metal-console
COPY id_rsa /id_rsa
CMD ["/metal-console"]
