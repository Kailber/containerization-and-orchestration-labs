FROM golang:1 AS build
WORKDIR /src
COPY go.mod ./
COPY api/main.go ./api/main.go
RUN CGO_ENABLED=0 go build -o /out/api ./api
ENTRYPOINT ["/out/api"]

FROM scratch AS runtime
COPY --from=build /out/api /api
ENTRYPOINT ["/api"]