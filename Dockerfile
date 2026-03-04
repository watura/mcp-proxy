FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mcp-proxy ./cmd/mcp-proxy

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /mcp-proxy /usr/local/bin/mcp-proxy
ENTRYPOINT ["mcp-proxy"]
