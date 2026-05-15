FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/game ./cmd/server/game

FROM alpine:3.22
RUN apk add --no-cache ca-certificates iputils net-tools procps
WORKDIR /app
COPY --from=builder /out/game /usr/local/bin/game
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/game"]
