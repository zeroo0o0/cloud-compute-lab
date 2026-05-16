FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY exp7/game-app/go.mod ./
RUN go mod download

COPY exp7/game-app/ ./

ARG TARGET=all
RUN if [ "$TARGET" = "game" ]; then \
      go build -o /out/game ./cmd/server/game; \
    elif [ "$TARGET" = "loadgen" ]; then \
      go build -o /out/loadgen ./cmd/loadgen; \
    else \
      go build -o /out/game ./cmd/server/game && \
      go build -o /out/loadgen ./cmd/loadgen; \
    fi

FROM alpine:3.22

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/ /app/
USER app
