# Stage 1: Build the React frontend
FROM node:20-alpine AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# Stage 2: Build the Go binary
FROM golang:1.22-alpine AS go-builder
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY go.mod ./
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /haberdasher \
    ./cmd/haberdasher

# Stage 3: Minimal runtime with Caddy
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl

RUN ARCH=$(uname -m) && \
    case $ARCH in \
      x86_64) CADDY_ARCH="amd64" ;; \
      aarch64) CADDY_ARCH="arm64" ;; \
      armv7l) CADDY_ARCH="armv7" ;; \
      *) CADDY_ARCH="amd64" ;; \
    esac && \
    curl -fsSL "https://github.com/caddyserver/caddy/releases/download/v2.8.4/caddy_2.8.4_linux_${CADDY_ARCH}.tar.gz" \
    | tar -xz -C /usr/local/bin caddy && \
    chmod +x /usr/local/bin/caddy

COPY --from=go-builder /haberdasher /usr/local/bin/haberdasher

RUN mkdir -p /data/caddy

VOLUME ["/data"]

EXPOSE 80 443 8080

ENV HABERDASHER_DATA=/data
ENV HABERDASHER_LISTEN=:8080

ENTRYPOINT ["/usr/local/bin/haberdasher"]
