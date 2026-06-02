FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY exp6/game-app/go.mod ./
RUN go mod download

COPY exp6/game-app/ ./
RUN go build -o /out/game ./cmd/server/game

FROM alpine:3.22

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/game /app/game
USER app
