FROM golang:1.21 as builder
WORKDIR /work
COPY . .
RUN make

FROM alpine:3.19

COPY --from=builder /work/bin/metal-console /

CMD ["/metal-console"]
