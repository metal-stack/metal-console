FROM golang:1.26 AS builder
WORKDIR /work
COPY . .
RUN make

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=builder /work/bin/metal-console /
CMD ["/metal-console"]
