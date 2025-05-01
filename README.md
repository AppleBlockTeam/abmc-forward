# ABMC-Forward

一个为 [AppleBlock](https://appleblock.cn) 服务器开发基于 Golang 开发的高性能流量转发工具，支持 Proxy Protocol 协议，专为 Minecraft 服务器设计，同时支持 Java 版和基岩版。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.23.2-blue.svg)](https://golang.org/)

## 功能特点

- 双协议支持：同时支持 TCP 和 UDP 转发
- Minecraft 协议适配：针对 Minecraft Java 版（TCP）和基岩版（UDP）进行了特别优化
- Proxy Protocol 支持：可选择性启用 Proxy Protocol v1/v2，保留客户端真实 IP
- 高性能：采用 Go 语言并发模型，支持大量连接
- 双栈支持：同时支持 IPv4 和 IPv6 连接
- 应急模式：后端不可用时提供自定义 MOTD 和踢出信息

## 安装方法

### 从源码构建

1. 确保已安装 Go 1.23.2 或更高版本
2. 克隆仓库并编译

```bash
git clone https://github.com/AppleBlockTeam/abmc-forward.git
cd abmc-forward
go build .
```

## 快速开始

1. 创建配置文件

```bash
./abmc-forward -create-config
```

2. 编辑 `config.yaml` 文件，根据需求修改配置

3. 运行转发器

```bash
./abmc-forward
```

## 配置说明

配置文件 `config.yaml` 包含以下选项：

```yaml
# 本地监听地址
tcp_addr: "[::]:25565"      # Java 版 Minecraft (TCP)
udp_addr: "[::]:19132"      # 基岩版 Minecraft (UDP)

# 远程服务器地址
remote_tcp_addr: "127.0.0.1:25566"
remote_udp_addr: "127.0.0.1:19133"

# 使用的协议，可选值: tcp, udp, both
protocol: "both"

# 是否使用 Proxy Protocol
use_proxy_proto: true

# Proxy Protocol 版本 (1 或 2)
proxy_proto_ver: 2

# 为 UDP 连接禁用 Proxy Protocol (可选)
disable_udp_proxy_proto: false

# 连接超时时间，单位为秒（5分钟 = 300秒）
timeout: 300s

# 缓冲区大小，单位为字节（32KB = 32768字节）
buffer_size: 32768

# 是否记录连接信息
log_connections: true

# === 后端服务器不可用时的配置 ===

# 是否启用后端不可用模式
fallback_mode: true

# 后端不可用时显示的 MOTD 信息
fallback_motd: "§c服务器维护中..."

# 当玩家尝试进入不可用服务器时显示的踢出信息
fallback_kick_message: "§c服务器正在维护，请稍后再试！"
```

## 使用场景

### 反向代理

在前端放置 abmc-forward 代理，将流量转发到后端真实服务器：

```
玩家 → abmc-forward → Minecraft 服务器
```

## 许可证

ABMC-Forward 采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。
