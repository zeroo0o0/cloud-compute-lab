FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY cmd/game/main.go ./cmd/game/
RUN cd cmd/game && go build -o /game-server .

FROM alpine:3.19
RUN apk add --no-cache curl
COPY --from=builder /game-server /game-server
EXPOSE 8080
CMD ["/game-server"]
