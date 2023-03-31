FROM golang:1.20 as builder
WORKDIR /work
COPY . .
RUN make

FROM alpine:3.16

COPY --from=builder /work/bin/metal-console /

CMD ["/metal-console"]
