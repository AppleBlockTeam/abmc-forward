package server

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/AppleBlockTeam/abmc-forwarder/config"
	"github.com/AppleBlockTeam/abmc-forwarder/minecraft"
	"github.com/AppleBlockTeam/abmc-forwarder/proxy"
	"github.com/AppleBlockTeam/abmc-forwarder/utils"
)

// UDPHandler 处理UDP连接转发
type UDPHandler struct {
	config config.Config
	conn   net.PacketConn
	done   chan struct{}
	wg     *sync.WaitGroup
}

// NewUDPHandler 创建新的UDP处理器
func NewUDPHandler(cfg config.Config, wg *sync.WaitGroup, done chan struct{}) *UDPHandler {
	return &UDPHandler{
		config: cfg,
		wg:     wg,
		done:   done,
	}
}

// Start 启动UDP转发服务
func (h *UDPHandler) Start() error {
	var err error
	h.conn, err = net.ListenPacket("udp", h.config.UDPAddr)
	if err != nil {
		return err
	}

	log.Printf("UDP 转发服务已启动，监听于 %s，转发到 %s\n", h.config.UDPAddr, h.config.RemoteUDPAddr)

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.handlePackets()
	}()

	return nil
}

// Stop 停止UDP转发服务
func (h *UDPHandler) Stop() {
	if h.conn != nil {
		h.conn.Close()
	}
}

// handlePackets 处理UDP数据包
func (h *UDPHandler) handlePackets() {
	// UDP 连接映射
	connMap := make(map[string]*net.UDPConn)
	var connMapMutex sync.Mutex

	// 读取缓冲区
	buffer := make([]byte, h.config.BufferSize)

	// 检查是否专门为 UDP 禁用了 Proxy Protocol
	useProxyProto := h.config.UseProxyProto
	if h.config.DisableUDPProxyProto {
		useProxyProto = false
	}

	for {
		select {
		case <-h.done:
			// 关闭所有连接
			connMapMutex.Lock()
			for _, conn := range connMap {
				conn.Close()
			}
			connMapMutex.Unlock()
			return
		default:
		}

		// 设置读取超时
		if h.config.Timeout > 0 {
			h.conn.SetReadDeadline(time.Now().Add(h.config.Timeout))
		}

		// 读取 UDP 数据包
		n, clientAddr, err := h.conn.ReadFrom(buffer)
		if err != nil {
			// 检查是否是超时错误
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				continue
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Printf("读取 UDP 数据失败: %v\n", err)
			continue
		}

		// 如果缓冲区太小，输出警告
		if n >= h.config.BufferSize {
			log.Printf("警告: UDP 数据可能被截断，考虑增加缓冲区大小\n")
		}

		clientAddrStr := clientAddr.String()

		// 查找或创建到远程的连接
		connMapMutex.Lock()
		remoteConn, exists := connMap[clientAddrStr]

		if !exists {
			var err error
			remoteAddr, err := net.ResolveUDPAddr("udp", h.config.RemoteUDPAddr)
			if err != nil {
				log.Printf("解析远程地址失败: %v\n", err)
				connMapMutex.Unlock()
				continue
			}

			remoteConn, err = net.DialUDP("udp", nil, remoteAddr)
			if err != nil {
				log.Printf("[%s] 连接远程服务器失败: %v\n", h.config.RemoteUDPAddr, err)

				// 如果启用了后备模式，且连接失败，直接处理
				if h.config.FallbackMode {
					connMapMutex.Unlock()
					h.handleFallbackPacket(buffer[:n], clientAddr)
					continue
				}

				connMapMutex.Unlock()
				continue
			}

			connMap[clientAddrStr] = remoteConn

			if h.config.LogConnections {
				log.Printf("[%s] 新的 UDP 连接 -> [%s]\n", clientAddr, h.config.RemoteUDPAddr)
			}

			// 启动一个 goroutine 来处理远程服务器返回的响应
			h.wg.Add(1)
			go func(clientAddr net.Addr, remoteConn *net.UDPConn, clientAddrStr string) {
				defer h.wg.Done()
				defer func() {
					connMapMutex.Lock()
					delete(connMap, clientAddrStr)
					remoteConn.Close()
					connMapMutex.Unlock()
				}()

				responseBuffer := make([]byte, h.config.BufferSize)
				for {
					if h.config.Timeout > 0 {
						remoteConn.SetReadDeadline(time.Now().Add(h.config.Timeout))
					}

					// 从远程服务器读取响应
					n, _, err := remoteConn.ReadFrom(responseBuffer)
					if err != nil {
						netErr, ok := err.(net.Error)
						if ok && netErr.Timeout() {
							continue
						}
						if !utils.IsConnectionClosed(err) {
							log.Printf("从远程服务器读取 UDP 响应失败: %v\n", err)
						}
						break
					}

					// 获取到响应数据
					responseData := responseBuffer[:n]

					// 将响应数据发送给客户端
					_, err = h.conn.WriteTo(responseData, clientAddr)
					if err != nil && !utils.IsConnectionClosed(err) {
						log.Printf("向客户端写入 UDP 响应失败: %v\n", err)
						break
					}
				}
			}(clientAddr, remoteConn, clientAddrStr)
		}

		// 转发数据到远程服务器
		data := buffer[:n]

		// 如果需要使用 Proxy Protocol（UDP 也实现）
		if useProxyProto && !exists {
			var headerData []byte
			var err error

			// 根据 Proxy Protocol 版本选择不同的实现
			if h.config.ProxyProtoVer == 1 {
				// 对于 V1，使用文本格式
				headerData, err = proxy.GenerateProxyProtocolV1Header(clientAddr, h.conn.LocalAddr())
			} else {
				// 对于 V2，使用二进制格式
				headerData, err = proxy.WriteProxyProtocolUDP(clientAddr, h.conn.LocalAddr(), h.config.ProxyProtoVer)
			}

			if err != nil {
				log.Printf("生成 Proxy Protocol UDP 头失败: %v\n", err)
			} else {
				// 将 Proxy Protocol 头与数据合并
				combinedData := append(headerData, data...)
				data = combinedData
			}
		}
		connMapMutex.Unlock()

		_, err = remoteConn.Write(data)
		if err != nil && !utils.IsConnectionClosed(err) {
			log.Printf("转发 UDP 数据到远程服务器失败: %v\n", err)
		}
	}
}

// handleFallbackPacket 处理后备模式下的UDP数据包
func (h *UDPHandler) handleFallbackPacket(data []byte, clientAddr net.Addr) {
	// 解析接收到的数据包
	packet := minecraft.ParseBedrockPacket(data)
	if packet != nil {
		// 检查数据包类型，为不同类型提供不同的响应
		switch packet.PacketID {
		case minecraft.OpenConnectionRequest1, minecraft.OpenConnectionRequest2:
			// 客户端尝试建立连接，发送断开通知
			disconnectPacket := minecraft.GenerateDisconnectPacket(h.config.FallbackKickMessage)
			if _, err := h.conn.WriteTo(disconnectPacket, clientAddr); err != nil {
				log.Printf("发送断开通知失败: %v\n", err)
			}
			if h.config.LogConnections {
				log.Printf("[%s] 已发送基岩版断开连接通知\n", clientAddr)
			}

		case 0x01: // Unconnected Ping
			// 客户端发送状态请求，返回自定义MOTD
			customPong := minecraft.GenerateCustomPongPacket(
				h.config.FallbackMotd,
				100, // 默认最大玩家数
				0,   // 默认在线玩家数
			)
			if _, err := h.conn.WriteTo(customPong, clientAddr); err != nil {
				log.Printf("发送基岩版自定义状态响应失败: %v\n", err)
			}
			if h.config.LogConnections {
				log.Printf("[%s] 已发送基岩版自定义状态响应\n", clientAddr)
			}
		}
	}
}
