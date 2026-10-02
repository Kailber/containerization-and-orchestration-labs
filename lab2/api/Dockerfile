FROM golang:1.26.8 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./

RUN CGO_ENABLED=0 go build -o /out/api .

FROM alpine:3.22

WORKDIR /app

COPY --from=builder /out/api ./api

EXPOSE 8080

USER 65532:65532

CMD ["./api"]