# ---------- Builder Stage ----------
FROM golang:1.23-alpine AS builder

# 获取目标平台信息
ARG TARGETPLATFORM
ARG GOARCH
RUN echo "Building for $TARGETPLATFORM"

# 根据目标平台设置正确的 GOARCH
RUN if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
      echo "GOARCH=amd64" >> /etc/environment; \
    elif [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
      echo "GOARCH=arm64" >> /etc/environment; \
    else \
      echo "Unsupported platform: $TARGETPLATFORM" && exit 1; \
    fi

WORKDIR /app

# 复制源代码
COPY . .

# 安装构建工具、编译
RUN . /etc/environment && \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o abmc-forwarder .

# ---------- Runtime Stage ----------
FROM scratch AS runtime

# 拷贝已压缩的静态二进制
COPY --from=builder /app/abmc-forwarder /abmc-forwarder

# 以非 root（UID/GID 1000）运行
USER 1000:1000

# 暴露端口与环境变量
EXPOSE 25565 25565
EXPOSE 19132 19132

ENTRYPOINT ["/abmc-forwarder"]