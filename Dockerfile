FROM golang:1.25 AS builder
WORKDIR /work
COPY . .
RUN make

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /work/bin/metal-console /
CMD ["/metal-console"]
