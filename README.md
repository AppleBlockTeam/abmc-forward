# abmc-forwarder

一个为 [AppleBlock](https://appleblock.cn) 服务器设计并开发基于 Golang 的高性能流量转发工具，支持 Proxy Protocol 协议，专为 Minecraft 服务器设计，同时支持 Java 版和基岩版。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.23.2-blue.svg)](https://golang.org/)

## 功能特点

- 双协议支持：同时支持 TCP 和 UDP 转发
- Minecraft 协议适配：针对 Minecraft Java 版（TCP）进行了特别优化
- Proxy Protocol 支持：可选择性启用 Proxy Protocol v1/v2，保留客户端真实 IP
- 高性能：采用 Go 语言并发模型，支持大量连接
- 双栈支持：同时支持 IPv4 和 IPv6 连接
- 应急模式：后端不可用时提供自定义 MOTD 和踢出信息（Only JAVA）

## 实现功能
 - [x] 支持 TCP
 - [x] 支持 UDP
 - [x] 支持 Proxy Protocol (TCP)
 - [x] 支持 Proxy Protocol (UDP)
 - [x] 支持连接不上 JE 后端时返回 MOTD 信息
 - [ ] 统一管理面板
 - [x] Docker

- [ ] ~~支持连接不上 BE 后端时返回 MOTD 信息~~(由于 UDP 无状态、无握手，无法单纯靠协议判断应用是否在跑，所以暂不支持)


## 安装方法

### 从源码构建

1. 确保已安装 Go 1.23.2 或更高版本
   
2. 克隆仓库并编译

```bash
git clone https://github.com/AppleBlockTeam/abmc-forwarder.git
cd abmc-forwarder
go build .
```

### Docker 部署
1. 准备配置文件 `config.yaml`，根据需求修改配置

2. 拉取并运行 Docker 容器

```bash
docker run -d \
  --name abmc-forwarder \
  -p 25565:25565 \
  -p 19132:19132/udp \
  -v $(pwd)/config.yaml:/config.yaml \
  mxmilu666/abmc-forwarder:latest -config /config.yaml
```

4. 使用 Docker Compose（可选）

创建 `docker-compose.yml` 文件：

```yaml
services:
  abmc-forwarder:
    image: mxmilu666/abmc-forwarder:latest
    container_name: abmc-forwarder
    restart: unless-stopped
    ports:
      - "25565:25565"
      - "19132:19132/udp"
    volumes:
      - ./config.yaml:/config.yaml
    command: -config /config.yaml
```

运行服务：

```bash
docker-compose up -d
```

## 快速开始

1. 创建配置文件

```bash
./abmc-forwarder -create-config
```

2. 编辑 `config.yaml` 文件，根据需求修改配置

3. 运行转发器

```bash
./abmc-forwarder
```

## 配置说明

配置文件 `config.yaml` 包含以下选项：

```yaml
# 同时监听 IPv4 和 IPv6（双栈模式）
tcp_addr: "[::]:25565"
udp_addr: "[::]:19132"
remote_tcp_addr: "127.0.0.1:25566"
remote_udp_addr: "127.0.0.1:19133"

# 使用的协议，可选值: tcp, udp, both
protocol: "both"

# 是否使用 Proxy Protocol
use_proxy_proto: true

# Proxy Protocol 版本 (1 或 2)
proxy_proto_ver: 2

# 连接超时时间，单位为秒（5分钟 = 300秒）
timeout: 300s

# 缓冲区大小，单位为字节（32KB = 32768字节）
buffer_size: 32768

# 是否记录连接信息
log_connections: true

# ====== 后端服务器不可用时的配置 ======

# 是否启用后端不可用模式（当后端服务器连接失败时启用）
fallback_mode: true

# 后端不可用时显示的 MOTD 信息
fallback_motd: "§cABMC-Forwarder 服务器维护中..."

# 当玩家尝试进入不可用服务器时显示的踢出信息
fallback_kick_message: "§c服务器正在维护，请稍后再试！"

# fallback 使用的协议
version: "1.20.4"
protocol_version: "765"

# fallback 的在线人数和最大人数
players: 
  max: 1919810
  online: 114514
```

## 许可证

abmc-forwarder 采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。
