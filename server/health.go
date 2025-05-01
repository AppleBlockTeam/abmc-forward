package server

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/AppleBlockTeam/abmc-forward/config"
)

// BackendStatus 表示后端服务器的状态
type BackendStatus struct {
	TCPAvailable bool      // TCP 后端是否可用
	UDPAvailable bool      // UDP 后端是否可用
	LastChecked  time.Time // 最后检查时间
}

// HealthChecker 负责检查后端服务器的健康状态
type HealthChecker struct {
	config *config.Config
	mu     sync.RWMutex
	status BackendStatus
}

// NewHealthChecker 创建新的健康检查器
func NewHealthChecker(cfg *config.Config) *HealthChecker {
	return &HealthChecker{
		config: cfg,
	}
}

// CheckTCPBackendAvailable 检查TCP后端是否可用
func (h *HealthChecker) CheckTCPBackendAvailable() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查TCP后端
	if h.config.Protocol == "tcp" || h.config.Protocol == "both" {
		h.status.TCPAvailable = checkTCPConnection(h.config.RemoteTCPAddr, 3*time.Second)
		h.status.LastChecked = time.Now()

		// 仅记录不可用状态
		if h.config.LogConnections && !h.status.TCPAvailable {
			log.Printf("TCP后端状态: %v (%s)\n", h.status.TCPAvailable, h.config.RemoteTCPAddr)
		}
	}

	return h.status.TCPAvailable
}

// CheckUDPBackendAvailable 检查UDP后端是否可用
func (h *HealthChecker) CheckUDPBackendAvailable() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查UDP后端
	if h.config.Protocol == "udp" || h.config.Protocol == "both" {
		h.status.UDPAvailable = checkUDPConnection(h.config.RemoteUDPAddr, 3*time.Second)
		h.status.LastChecked = time.Now()

		// 仅记录不可用状态
		if h.config.LogConnections && !h.status.UDPAvailable {
			log.Printf("UDP后端状态: %v (%s)\n", h.status.UDPAvailable, h.config.RemoteUDPAddr)
		}
	}

	return h.status.UDPAvailable
}

// checkTCPConnection 检查TCP连接是否可用
func checkTCPConnection(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkUDPConnection 检查UDP连接是否可用
func checkUDPConnection(addr string, timeout time.Duration) bool {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return false
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return false
	}
	defer conn.Close()

	// 设置读写超时
	conn.SetDeadline(time.Now().Add(timeout))

	// 尝试发送一个简单的数据包
	_, err = conn.Write([]byte{0x00}) // 发送单字节数据包
	if err != nil {
		return false
	}

	return true
}
