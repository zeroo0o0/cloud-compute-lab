# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/node ./cmd/node
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/client ./cmd/client
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/admin ./cmd/admin

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/ /usr/local/bin/
RUN mkdir -p /data/cold /data/hot/checkpoints
EXPOSE 9310 9311 9312 9313
ENTRYPOINT ["server"]
