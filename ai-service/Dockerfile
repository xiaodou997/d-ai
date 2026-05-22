# ========================
# 二进制部署专用 Dockerfile
# 使用方式：
#   1. make build          # 先编译出 bin/server-amd64
#   2. cp bin/server-amd64 ./ai-server
#   3. docker build -t ai-service .
# ========================

FROM alpine:3.19

WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata

# 复制预编译的二进制文件
COPY ai-server /app/ai-server

# 验证二进制文件存在
RUN test -f /app/ai-server || (echo "Error: ai-server binary not found" && exit 1)
RUN chmod +x /app/ai-server

# 复制数据库初始化脚本
COPY db/ /app/db/

# 创建日志目录
RUN mkdir -p /app/logs

# 暴露端口
EXPOSE 13010

# 运行
ENTRYPOINT ["/app/ai-server"]
