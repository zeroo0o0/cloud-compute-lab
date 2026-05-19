FROM golang:1.23-bullseye AS builder
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/game ./cmd/server/game

FROM python:3.11-slim
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates iputils-ping net-tools procps \
	&& rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /out/game /usr/local/bin/game
ENV PYTHON_BIN=python
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/game"]
