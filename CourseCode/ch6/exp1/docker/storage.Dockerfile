FROM golang:1.25-alpine AS builder
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/storage ./cmd/server/storage

FROM alpine:3.22
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates iputils net-tools procps
WORKDIR /app
COPY --from=builder /out/storage /usr/local/bin/storage
VOLUME ["/app/data"]
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/storage"]
