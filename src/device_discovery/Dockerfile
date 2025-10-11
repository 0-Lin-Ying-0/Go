# ============ Builder ============
FROM golang:1.22 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/discovery ./cmd/discovery

# ============ Runner ============
FROM debian:stable-slim

# 安装 ping 工具（ICMPScanner 需要）
RUN apt-get update && apt-get install -y --no-install-recommends iputils-ping ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /bin/discovery /app/discovery

EXPOSE 8080
ENTRYPOINT ["/app/discovery"]
