FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/game-map0 ./cmd/env_demo

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY docker/config.yaml /etc/game/config.yaml
COPY --from=builder /out/game-map0 /usr/local/bin/game-map0
ENV CONFIG_PATH=/etc/game/config.yaml
ENV LOG_DIR=/app/data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/game-map0"]
