# syntax=docker/dockerfile:1

FROM registry.cn-hangzhou.aliyuncs.com/library/golang:1.22-alpine AS builder
ENV GOPROXY=https://goproxy.cn,direct GO111MODULE=on CGO_ENABLED=0 GOOS=linux
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/discovery ./cmd/discovery

FROM registry.cn-hangzhou.aliyuncs.com/library/alpine:3.20
WORKDIR /app
COPY --from=builder /out/discovery /app/discovery
EXPOSE 8080
ENTRYPOINT ["/app/discovery"]
