FROM golang:1.26.8 AS builder
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/batch .

FROM scratch
COPY --from=builder /out/batch /batch
USER 65532:65532
ENTRYPOINT ["/batch"]
