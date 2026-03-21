package server

import (
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AppleBlockTeam/abmc-forwarder/config"
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

type udpSession struct {
	conn       *net.UDPConn
	lastActive atomic.Int64
}

func newUDPSession(conn *net.UDPConn) *udpSession {
	session := &udpSession{conn: conn}
	session.touch()
	return session
}

func (s *udpSession) touch() {
	s.lastActive.Store(time.Now().UnixNano())
}

func (s *udpSession) idleFor(now time.Time) time.Duration {
	lastActive := s.lastActive.Load()
	if lastActive == 0 {
		return 0
	}

	return now.Sub(time.Unix(0, lastActive))
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
	connMap := make(map[string]*udpSession)
	var connMapMutex sync.Mutex

	// 读取缓冲区
	buffer := make([]byte, h.config.BufferSize)

	// 检查是否专门为 UDP 禁用了 Proxy Protocol
	useProxyProto := h.config.UseProxyProto
	if h.config.DisableUDPProxyProto {
		useProxyProto = false
	}

	cleanupInterval := time.Duration(0)
	if h.config.Timeout > 0 {
		cleanupInterval = h.config.Timeout
		if cleanupInterval > time.Minute {
			cleanupInterval = time.Minute
		}
		if cleanupInterval < time.Second {
			cleanupInterval = time.Second
		}
	}

	cleanupIdleSessions := func(now time.Time) {
		if h.config.Timeout <= 0 {
			return
		}

		expiredSessions := make([]*udpSession, 0)

		connMapMutex.Lock()
		for clientAddrStr, session := range connMap {
			if session.idleFor(now) < h.config.Timeout {
				continue
			}

			delete(connMap, clientAddrStr)
			expiredSessions = append(expiredSessions, session)

			if h.config.LogConnections {
				log.Printf("[%s] UDP 会话空闲超时，已关闭\n", clientAddrStr)
			}
		}
		connMapMutex.Unlock()

		for _, session := range expiredSessions {
			session.conn.Close()
		}
	}

	for {
		select {
		case <-h.done:
			// 关闭所有连接
			sessions := make([]*udpSession, 0)
			connMapMutex.Lock()
			for _, session := range connMap {
				sessions = append(sessions, session)
			}
			connMapMutex.Unlock()

			for _, session := range sessions {
				session.conn.Close()
			}
			return
		default:
		}

		// 设置读取超时
		if cleanupInterval > 0 {
			h.conn.SetReadDeadline(time.Now().Add(cleanupInterval))
		}

		// 读取 UDP 数据包
		n, clientAddr, err := h.conn.ReadFrom(buffer)
		if err != nil {
			// 检查是否是超时错误
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				cleanupIdleSessions(time.Now())
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
		session, exists := connMap[clientAddrStr]

		if !exists {
			var err error
			remoteAddr, err := net.ResolveUDPAddr("udp", h.config.RemoteUDPAddr)
			if err != nil {
				log.Printf("解析远程地址失败: %v\n", err)
				connMapMutex.Unlock()
				continue
			}

			remoteConn, err := net.DialUDP("udp", nil, remoteAddr)
			if err != nil {
				log.Printf("[%s] 连接远程服务器失败: %v\n", h.config.RemoteUDPAddr, err)
				connMapMutex.Unlock()
				continue
			}

			session = newUDPSession(remoteConn)
			connMap[clientAddrStr] = session

			if h.config.LogConnections {
				log.Printf("[%s] 新的 UDP 连接 -> [%s]\n", clientAddr, h.config.RemoteUDPAddr)
			}

			// 启动一个 goroutine 来处理远程服务器返回的响应
			h.wg.Add(1)
			go func(clientAddr net.Addr, session *udpSession, clientAddrStr string) {
				defer h.wg.Done()
				defer func() {
					connMapMutex.Lock()
					if currentSession, ok := connMap[clientAddrStr]; ok && currentSession == session {
						delete(connMap, clientAddrStr)
					}
					connMapMutex.Unlock()

					session.conn.Close()
				}()

				responseBuffer := make([]byte, h.config.BufferSize)
				for {
					if cleanupInterval > 0 {
						session.conn.SetReadDeadline(time.Now().Add(cleanupInterval))
					}

					// 从远程服务器读取响应
					n, _, err := session.conn.ReadFrom(responseBuffer)
					if err != nil {
						netErr, ok := err.(net.Error)
						if ok && netErr.Timeout() {
							if h.config.Timeout > 0 && session.idleFor(time.Now()) >= h.config.Timeout {
								break
							}
							continue
						}
						if !utils.IsConnectionClosed(err) {
							log.Printf("从远程服务器读取 UDP 响应失败: %v\n", err)
						}
						break
					}

					// 获取到响应数据
					responseData := responseBuffer[:n]
					session.touch()

					// 将响应数据发送给客户端
					_, err = h.conn.WriteTo(responseData, clientAddr)
					if err != nil && !utils.IsConnectionClosed(err) {
						log.Printf("向客户端写入 UDP 响应失败: %v\n", err)
						break
					}
				}
			}(clientAddr, session, clientAddrStr)
		}

		session.touch()
		remoteConn := session.conn

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

		cleanupIdleSessions(time.Now())
	}
}
