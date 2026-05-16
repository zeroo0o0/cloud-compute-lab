FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY exp5/game-app/go.mod ./
RUN go mod download

COPY exp5/game-app/ ./

ARG TARGET=all
RUN if [ "$TARGET" = "gateway" ]; then \
      go build -o /out/gateway ./cmd/server/gateway; \
    elif [ "$TARGET" = "game" ]; then \
      go build -o /out/game ./cmd/server/game; \
    elif [ "$TARGET" = "storage" ]; then \
      go build -o /out/storage ./cmd/server/storage; \
    else \
      go build -o /out/gateway ./cmd/server/gateway && \
      go build -o /out/game ./cmd/server/game && \
      go build -o /out/storage ./cmd/server/storage; \
    fi

FROM alpine:3.22

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/ /app/
USER app
