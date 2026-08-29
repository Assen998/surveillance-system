# 构建阶段
FROM golang:1.22-alpine AS builder

# 安装构建依赖
RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /build

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=1.0.0 -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.GitCommit=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')" \
    -o surveillance-server ./cmd/server

# 运行阶段
FROM alpine:3.19

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    ffmpeg \
    tzdata \
    curl \
    && rm -rf /var/cache/apk/*

# 创建非 root 用户
RUN addgroup -g 1000 -S surveillance && \
    adduser -u 1000 -S surveillance -G surveillance

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/surveillance-server .

# 复制配置文件模板
COPY --from=builder /build/configs ./configs

# 创建必要目录
RUN mkdir -p /app/data /app/recordings /app/logs /app/models && \
    chown -R surveillance:surveillance /app

# 切换到非 root 用户
USER surveillance

# 暴露端口
EXPOSE 8080 8081 9090

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# 启动命令
ENTRYPOINT ["./surveillance-server"]
CMD ["./configs/config.yaml"]