FROM golang:1.26.8 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY services/ ./services/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./services/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./services/worker


FROM scratch AS api

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/api /api

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/api"]


FROM scratch AS worker

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/worker /worker

USER 65532:65532

EXPOSE 8082

ENTRYPOINT ["/worker"]