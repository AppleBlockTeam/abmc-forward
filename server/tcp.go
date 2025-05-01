package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
			log.Printf("[无IP] 接受 TCP 连接失败: %v\n", err)
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

	ip := clientConn.RemoteAddr().String()

	// 直接尝试连接远程服务器
	remoteConn, err := net.DialTimeout("tcp", h.config.RemoteTCPAddr, h.config.Timeout)
	if err != nil {
		log.Printf("[%s] 连接远程服务器失败 %s: %v\n", ip, h.config.RemoteTCPAddr, err)

		// 只有在启用了后备模式时才处理失败情况
		if h.config.FallbackMode {
			if h.config.LogConnections {
				log.Printf("[%s] TCP后端不可用，使用后备模式处理连接\n", ip)
			}

			h.handleFallbackMode(clientConn)
		}
		return
	}
	defer remoteConn.Close()

	// 如果启用 ProxyProtocol 并且是 TCP 连接
	if h.config.UseProxyProto {
		err := proxy.WriteProxyProtocolHeader(clientConn.RemoteAddr(), clientConn.LocalAddr(), h.config.ProxyProtoVer, remoteConn)
		if err != nil {
			log.Printf("[%s] 写入 Proxy Protocol 头失败: %v\n", ip, err)
			return
		}
	}

	if h.config.LogConnections {
		log.Printf("[%s] 新的 TCP 连接 -> %s\n", ip, h.config.RemoteTCPAddr)
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
					log.Printf("[%s] 从客户端读取数据失败: %v\n", ip, err)
				}
				break
			}

			// 将数据转发到远程服务器
			_, err = remoteConn.Write(buf[:n])
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("[%s] 向远程服务器发送数据失败: %v\n", ip, err)
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
					log.Printf("[%s] 从远程服务器读取数据失败: %v\n", ip, err)
				}
				break
			}

			// 将数据转发给客户端
			_, err = clientConn.Write(buf[:n])
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("[%s] 向客户端发送数据失败: %v\n", ip, err)
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

// handleFallbackMode 处理后备模式下的连接，实现完整的服务器列表 ping 协议
func (h *TCPHandler) handleFallbackMode(clientConn net.Conn) {

	ip := clientConn.RemoteAddr().String()

	// 读取初始数据包
	buf := make([]byte, h.config.BufferSize)
	n, err := clientConn.Read(buf)
	if err != nil {
		if !utils.IsConnectionClosed(err) {
			log.Printf("[%s] 从客户端读取数据失败: %v\n", ip, err)
		}
		return
	}

	// 兼容旧版 Server List Ping (0xFE)
	if n > 0 && buf[0] == 0xFE {
		motd := h.config.FallbackMotd
		protocol := h.config.ProtocolVersion
		if protocol == "" {
			protocol = "127" // 默认
		}
		version := h.config.Version
		if version == "" {
			version = "1.19.3"
		}
		online := "0"
		max := "100"
		resp := fmt.Sprintf("§1\x00%s\x00%s\x00%s\x00%s\x00%s", protocol, version, motd, online, max)
		utf16 := encodeUTF16BE(resp)
		var b bytes.Buffer
		b.WriteByte(0xFF)
		_ = binary.Write(&b, binary.BigEndian, uint16(len(utf16)))
		for _, r := range utf16 {
			_ = binary.Write(&b, binary.BigEndian, r)
		}
		clientConn.Write(b.Bytes())
		return
	}

	// 尝试解析数据包
	packet, err := minecraft.ParseJavaPacket(buf[:n])
	if err != nil {
		log.Printf("[%s] 解析Java包失败: %v\n", ip, err)
		return
	}

	// 如果是握手包，解析以确定下一状态
	if packet.PacketID == 0x00 {
		handshake, err := minecraft.ParseHandshakePacket(packet.Data)
		if err != nil {
			log.Printf("[%s] 解析握手包失败: %v\n", ip, err)
			return
		}

		// 如果是状态请求（服务器列表查询）
		if handshake.NextState == minecraft.StatusState {
			// 读取下一个数据包 (状态请求)
			clientConn.SetReadDeadline(time.Now().Add(3 * time.Second)) // 设置读取超时
			n, err = clientConn.Read(buf)
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("[%s] 读取状态请求失败: %v\n", ip, err)
				}
				return
			}

			// 验证这是否是一个状态请求包
			statusPacket, err := minecraft.ParseJavaPacket(buf[:n])
			if err != nil {
				log.Printf("[%s] 解析状态请求包失败: %v\n", ip, err)
				return
			}

			// 在某些客户端实现中，状态请求可能使用不同的包ID，所以我们宽容一些
			if statusPacket.PacketID != minecraft.JavaStatusRequest {
				log.Printf("[%s] 警告: 状态请求包ID异常 (0x%02x)，但将继续处理", ip, statusPacket.PacketID)
			}

			// 构建自定义状态响应
			motd := h.config.FallbackMotd

			// 创建默认的响应数据
			version := h.config.Version
			if version == "" {
				version = "1.19.3"
			}
			protocol := h.config.ProtocolVersion
			if protocol == "" {
				protocol = "761"
			}
			defaultData := []byte(fmt.Sprintf(`{"version":{"name":"%s","protocol":%s},"players":{"max":100,"online":0,"sample":[]},"description":{"text":"服务器暂时不可用"},"favicon":"","enforcesSecureChat":true}`, version, protocol))

			statusData, err := minecraft.ModifyJavaStatusResponse(
				defaultData, // 使用预定义的基础模板
				motd,
				100, // 默认最大玩家数
				0,   // 默认在线玩家数
			)
			if err != nil {
				log.Printf("[%s] 创建状态响应失败: %v", ip, err)
				return
			}

			// 构造响应包
			var responsePacket bytes.Buffer

			// 首先写入 JSON 字符串的长度
			jsonLenBytes := minecraft.WriteVarInt(int32(len(statusData)))

			// 计算总包长度: JSON长度字段 + JSON数据 + 包ID(1字节)
			packetLength := len(jsonLenBytes) + len(statusData) + 1

			// 写入总包长度
			responsePacket.Write(minecraft.WriteVarInt(int32(packetLength)))

			// 写入包ID (状态响应包ID是 0x00)
			responsePacket.WriteByte(0x00)

			// 写入JSON长度
			responsePacket.Write(jsonLenBytes)

			// 写入JSON数据
			responsePacket.Write(statusData)

			// 设置写入超时并发送响应
			clientConn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			_, err = clientConn.Write(responsePacket.Bytes())
			if err != nil {
				log.Printf("[%s] 发送状态响应失败: %v\n", ip, err)
				return
			}

			// 读取可能的ping请求并响应
			clientConn.SetReadDeadline(time.Now().Add(5 * time.Second)) // 等待ping请求的时间更长些
			n, err = clientConn.Read(buf)
			if err != nil {
				if err != io.EOF && !utils.IsConnectionClosed(err) {
					log.Printf("[%s] 读取ping请求失败: %v\n", ip, err)
				}
				return
			}

			pingPacket, err := minecraft.ParseJavaPacket(buf[:n])
			if err == nil && pingPacket.PacketID == 0x01 {
				// 这是一个ping请求，发送相同的payload作为响应
				pingResponse := minecraft.GeneratePingResponse(pingPacket.Data)

				clientConn.SetWriteDeadline(time.Now().Add(3 * time.Second))
				_, err = clientConn.Write(pingResponse)
				if err != nil {
					log.Printf("[%s] 发送ping响应失败: %v\n", ip, err)
				} else {
					log.Printf("[%s] 成功发送ping响应", ip)
				}
			} else {
				if err != nil {
					log.Printf("[%s] 解析ping包失败: %v", ip, err)
				} else {
					log.Printf("[%s] 收到的包不是ping请求，PacketID: 0x%02x", ip, pingPacket.PacketID)
				}
			}
		} else if handshake.NextState == minecraft.LoginState {
			// 这是一个登录请求，发送断开连接消息
			clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, err = clientConn.Read(buf)
			if err != nil {
				if !utils.IsConnectionClosed(err) {
					log.Printf("[%s] 读取登录请求失败: %v\n", ip, err)
				}
				return
			}

			// 发送登录拒绝数据包
			kickPacket := minecraft.GenerateLoginDenyPacket(h.config.FallbackKickMessage)

			clientConn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			sentBytes, err := clientConn.Write(kickPacket)
			if err != nil {
				log.Printf("[%s] 发送踢出消息失败: %v\n", ip, err)
			} else {
				log.Printf("[%s] 成功发送登录拒绝包，共 %d 字节，原因: \"%s\"", ip, sentBytes, h.config.FallbackKickMessage)
			}
		}
	}
}

// encodeUTF16BE 将字符串编码为 UTF-16BE []uint16
func encodeUTF16BE(s string) []uint16 {
	runes := []rune(s)
	out := make([]uint16, len(runes))
	for i, r := range runes {
		out[i] = uint16(r)
	}
	return out
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 全局服务器实例
var globalServer *Server

// SetGlobalServer 设置全局服务器实例
func SetGlobalServer(server *Server) {
	globalServer = server
}
