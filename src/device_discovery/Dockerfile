# 多阶段构建：第一阶段编译 Go
FROM dockerproxy.com/library/golang:1.22-alpine AS builder

# 使用国内 Go 模块代理以加速依赖下载
ENV GOPROXY=https://goproxy.cn,direct \
    GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

# 复制 Go 模块文件并提前下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制项目源代码并编译二进制
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/discovery ./cmd/discovery

# 第二阶段：使用极简镜像运行二进制，可选择 alpine 或 scratch
# 如果需要 shell 或其他系统工具，则使用 alpine
FROM alpine:3.20

WORKDIR /app

# 从 builder 复制编译好的二进制
COPY --from=builder /out/discovery /app/discovery

EXPOSE 8080
ENTRYPOINT ["/app/discovery"]
