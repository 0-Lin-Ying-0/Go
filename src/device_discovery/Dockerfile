# 使用国内镜像源的多阶段构建 Dockerfile（Go 项目）
FROM dockerproxy.com/library/golang:1.22-alpine AS builder

# 设置Go环境：使用国内模块代理，加速依赖下载；禁用CGO以生成静态二进制
ENV GOPROXY=https://goproxy.cn,direct GO111MODULE=on CGO_ENABLED=0 GOOS=linux

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/discovery ./cmd/discovery

FROM dockerproxy.com/library/alpine:3.18

WORKDIR /app
COPY --from=builder /out/discovery /app/discovery

# 如有运行时所需的配置文件等，也可在此COPY进去
EXPOSE 8080
ENTRYPOINT ["/app/discovery"]
