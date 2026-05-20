FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY exp8/game-app/go.mod ./
RUN go mod download

COPY exp8/game-app/ ./

ARG TARGET=all
RUN if [ "$TARGET" = "gateway" ]; then \
      go build -o /out/gateway ./cmd/server/gateway; \
    else \
      go build -o /out/gateway ./cmd/server/gateway; \
    fi

FROM alpine:3.22

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/ /app/
USER app
