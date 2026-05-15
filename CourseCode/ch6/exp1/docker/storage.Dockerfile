FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/storage ./cmd/server/storage

FROM alpine:3.22
RUN apk add --no-cache ca-certificates iputils net-tools procps
WORKDIR /app
COPY --from=builder /out/storage /usr/local/bin/storage
VOLUME ["/app/data"]
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/storage"]
