package server

import (
	"bytes"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/AppleBlockTeam/abmc-forward/config"
	"github.com/AppleBlockTeam/abmc-forward/minecraft"
	"github.com/AppleBlockTeam/abmc-forward/proxy"
	"github.com/AppleBlockTeam/abmc-forward/utils"
)

// TCPHandler 处理TCP连接转发
type TCPHandler struct {
	config config.Config
	server net.Listener
	done   chan struct{}
	wg     *sync.WaitGroup
}

// NewTCPHandler 创建新的TCP处理器
func NewTCPHandler(cfg config.Config, wg *sync.WaitGroup, done chan struct{}) *TCPHandler {
	return &TCPHandler{
		config: cfg,
		wg:     wg,
		done:   done,
	}
}

// Start 启动TCP转发服务
func (h *TCPHandler) Start() error {
	var err error
	h.server, err = net.Listen("tcp", h.config.TCPAddr)
	if err != nil {
		return err
	}

	// 如果启用 Proxy Protocol
	if h.config.UseProxyProto {
		h.server = proxy.WrapListener(h.server, h.config.ProxyProtoVer)
	}

	log.Printf("TCP 转发服务已启动，监听于 %s，转发到 %s\n", h.config.TCPAddr, h.config.RemoteTCPAddr)

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.handleConnections()
	}()

	return nil
}

// Stop 停止TCP转发服务
func (h *TCPHandler) Stop() {
	if h.server != nil {
		h.server.Close()
	}
}

// handleConnections 处理TCP连接
func (h *TCPHandler) handleConnections() {
	for {
		select {
		case <-h.done:
			return
		default:
		}

		// 设置接受连接的超时时间
		if h.config.Timeout > 0 {
			// 检查是否是原始的TCPListener或已经包装的proxyproto.Listener
			if tcpListener, ok := h.server.(*net.TCPListener); ok {
				tcpListener.SetDeadline(time.Now().Add(h.config.Timeout))
			} else {
				// 当使用了Proxy Protocol时，不能直接设置超时
				// 在此情况下，超时将通过Accept方法的内部实现来处理
				// 这可能导致超时处理略有不同，但不会影响主要功能
			}
		}

		// 接受客户端连接
		clientConn, err := h.server.Accept()
		if err != nil {
			// 检查是否是超时错误
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				continue
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Printf("接受 TCP 连接失败: %v\n", err)
			continue
		}

		// 异步处理连接
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.handleConnection(clientConn)
		}()
	}
}

// handleConnection 处理单个TCP连接
func (h *TCPHandler) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// 从 Server 获取健康检查器
	healthChecker := h.getHealthChecker()

	// 如果启用了后端不可用模式，首先检查后端是否可用
	var backendAvailable bool = true
	if h.config.FallbackMode && healthChecker != nil {
		// 现在检查后端状态
		backendAvailable = healthChecker.CheckTCPBackendAvailable()

		// 如果后端不可用，使用自定义处理
		if !backendAvailable {
			if h.config.LogConnections {
				log.Printf("TCP后端不可用，使用后备模式处理连接: %s\n", clientConn.RemoteAddr())
			}

			// 读取初始数据包
			buf := make([]byte, h.config.BufferSize)
			n, err := clientConn.Read(buf)
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("从客户端读取数据失败: %v\n", err)
				}
				return
			}

			// 尝试解析数据包
			packet, err := minecraft.ParseJavaPacket(buf[:n])
			if err != nil {
				log.Printf("解析Java包失败: %v\n", err)
				return
			}

			// 如果是握手包，解析以确定下一状态
			if packet.IsHandshake() {
				handshake, err := minecraft.ParseHandshakePacket(packet.Data)
				if err != nil {
					log.Printf("解析握手包失败: %v\n", err)
					return
				}

				// 如果是状态请求（服务器列表查询）
				if handshake.NextState == minecraft.StatusState {
					// 读取下一个数据包 (状态请求)
					n, err = clientConn.Read(buf)
					if err != nil {
						if !utils.IsConnectionClosed(err) {
							log.Printf("读取状态请求失败: %v\n", err)
						}
						return
					}

					// 验证这是否是一个状态请求包
					statusPacket, err := minecraft.ParseJavaPacket(buf[:n])
					if err != nil || statusPacket.PacketID != minecraft.JavaStatusRequest {
						log.Printf("无效的状态请求包\n")
						return
					}

					// 构建自定义状态响应
					statusData, _ := minecraft.ModifyJavaStatusResponse(
						[]byte(`{"version":{"name":"1.19.3","protocol":761},"players":{"max":100,"online":0,"sample":[]},"description":{"text":""},"favicon":"","enforcesSecureChat":true}`),
						h.config.FallbackMotd,
						100, // 默认最大玩家数
						0,   // 默认在线玩家数
					)

					// 构造响应包
					var responsePacket bytes.Buffer
					packetLength := 1 + len(statusData) // 1字节的包ID + JSON数据长度
					responsePacket.Write(minecraft.WriteVarInt(int32(packetLength)))
					responsePacket.WriteByte(0x00) // 状态响应包ID
					responsePacket.Write(statusData)

					// 发送响应
					_, err = clientConn.Write(responsePacket.Bytes())
					if err != nil {
						log.Printf("发送状态响应失败: %v\n", err)
					}

					// 读取可能的ping请求并响应
					n, err = clientConn.Read(buf)
					if err != nil {
						if err != io.EOF && !utils.IsConnectionClosed(err) {
							log.Printf("读取ping请求失败: %v\n", err)
						}
						return
					}

					pingPacket, err := minecraft.ParseJavaPacket(buf[:n])
					if err == nil && pingPacket.PacketID == 0x01 {
						// 这是一个ping请求，发送相同的payload作为响应
						var pingResponse bytes.Buffer
						pingResponse.Write(minecraft.WriteVarInt(int32(1 + len(pingPacket.Data)))) // 长度
						pingResponse.WriteByte(0x01)                                               // Ping响应ID
						pingResponse.Write(pingPacket.Data)                                        // 时间戳payload

						_, err = clientConn.Write(pingResponse.Bytes())
						if err != nil {
							log.Printf("发送ping响应失败: %v\n", err)
						}
					}
				} else if handshake.NextState == minecraft.LoginState {
					// 这是一个登录请求，发送断开连接消息
					n, err = clientConn.Read(buf)
					if err != nil {
						if !utils.IsConnectionClosed(err) {
							log.Printf("读取登录请求失败: %v\n", err)
						}
						return
					}

					// 发送登录拒绝数据包
					kickPacket := minecraft.GenerateLoginDenyPacket(h.config.FallbackKickMessage)
					_, err = clientConn.Write(kickPacket)
					if err != nil {
						log.Printf("发送踢出消息失败: %v\n", err)
					}
				}

				return
			}

			// 如果不是握手包，直接关闭连接
			return
		}
	}

	// 正常处理模式 - 连接到远程服务器
	remoteConn, err := net.DialTimeout("tcp", h.config.RemoteTCPAddr, h.config.Timeout)
	if err != nil {
		log.Printf("连接远程服务器失败 %s: %v\n", h.config.RemoteTCPAddr, err)

		// 如果之前检测结果是可用，但实际连接失败了
		if backendAvailable && h.config.FallbackMode && healthChecker != nil {
			// 重新处理这个连接，这次将后端视为不可用
			backendAvailable = false
			h.handleConnection(clientConn)
		}

		return
	}
	defer remoteConn.Close()

	// 如果启用 ProxyProtocol 并且是 TCP 连接
	if h.config.UseProxyProto {
		err := proxy.WriteProxyProtocolHeader(clientConn.RemoteAddr(), clientConn.LocalAddr(), h.config.ProxyProtoVer, remoteConn)
		if err != nil {
			log.Printf("写入 Proxy Protocol 头失败: %v\n", err)
			return
		}
	}

	if h.config.LogConnections {
		log.Printf("新的 TCP 连接：%s -> %s\n", clientConn.RemoteAddr(), h.config.RemoteTCPAddr)
	}

	// 双向转发数据
	var wg sync.WaitGroup
	wg.Add(2)

	// 客户端 -> 远程
	go func() {
		defer wg.Done()
		buf := make([]byte, h.config.BufferSize)

		for {
			n, err := clientConn.Read(buf)
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("从客户端读取数据失败: %v\n", err)
				}
				break
			}

			// 将数据转发到远程服务器
			_, err = remoteConn.Write(buf[:n])
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("向远程服务器发送数据失败: %v\n", err)
				}
				break
			}
		}

		// 安全地关闭写入方向
		if tcpConn, ok := remoteConn.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// 远程 -> 客户端
	go func() {
		defer wg.Done()
		buf := make([]byte, h.config.BufferSize)

		for {
			n, err := remoteConn.Read(buf)
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("从远程服务器读取数据失败: %v\n", err)
				}
				break
			}

			// 将数据转发给客户端
			_, err = clientConn.Write(buf[:n])
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("向客户端发送数据失败: %v\n", err)
				}
				break
			}
		}

		// 安全地关闭写入方向
		if tcpConn, ok := clientConn.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// 等待双向转发完成
	wg.Wait()
}

// getHealthChecker 获取健康检查器
func (h *TCPHandler) getHealthChecker() *HealthChecker {
	// 获取父服务器
	if server, ok := getServerFromHandler(h); ok {
		return server.GetHealthChecker()
	}
	return nil
}

// getServerFromHandler 从处理器获取服务器实例
func getServerFromHandler(handler interface{}) (*Server, bool) {
	// 这是一个辅助函数，用于获取TCPHandler或UDPHandler所属的服务器实例
	// 由于Go没有类似于"parent"的概念，我们需要通过一些方式间接获取
	// 这里使用了一个全局变量来存储服务器实例
	// 实际使用时，您可能需要使用更优雅的方式，例如依赖注入

	// serverRegistry 是一个全局服务器注册表
	// 它应该在 main.go 中初始化并注入到服务器组件中
	if globalServer != nil {
		return globalServer, true
	}
	return nil, false
}

// 全局服务器实例
var globalServer *Server

// SetGlobalServer 设置全局服务器实例
func SetGlobalServer(server *Server) {
	globalServer = server
}
