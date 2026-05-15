FROM golang:1.22-alpine AS gobuilder
WORKDIR /app
COPY go.mod ./
COPY cmd/runtime/main.go ./cmd/runtime/
RUN cd cmd/runtime && go build -o /faas-runtime .

FROM python:3.12-slim
RUN pip install --no-cache-dir flask 2>/dev/null || true
COPY --from=gobuilder /faas-runtime /faas-runtime
COPY functions/ /app/functions/
WORKDIR /app
EXPOSE 9000
CMD ["/faas-runtime"]
