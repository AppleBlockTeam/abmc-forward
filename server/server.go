package server

import (
	"log"
	"sync"

	"github.com/AppleBlockTeam/abmc-forward/config"
)

// Server 表示转发服务器
type Server struct {
	tcpHandler *TCPHandler
	udpHandler *UDPHandler
	config     config.Config
	wg         sync.WaitGroup
	done       chan struct{}
}

// New 创建新的服务器实例
func New(cfg config.Config) *Server {
	done := make(chan struct{})
	server := &Server{
		config: cfg,
		done:   done,
	}

	// 创建TCP处理器
	if cfg.Protocol == "tcp" || cfg.Protocol == "both" {
		server.tcpHandler = NewTCPHandler(cfg, &server.wg, done)
	}

	// 创建UDP处理器
	if cfg.Protocol == "udp" || cfg.Protocol == "both" {
		server.udpHandler = NewUDPHandler(cfg, &server.wg, done)
	}

	return server
}

// Start 启动转发服务器
func (s *Server) Start() error {
	// 启动TCP转发服务
	if s.tcpHandler != nil {
		if err := s.tcpHandler.Start(); err != nil {
			return err
		}
	}

	// 启动UDP转发服务
	if s.udpHandler != nil {
		if err := s.udpHandler.Start(); err != nil {
			return err
		}
	}

	log.Println("转发服务器已启动")
	return nil
}

// Stop 停止转发服务器
func (s *Server) Stop() {
	// 通知所有处理器停止
	close(s.done)

	// 停止TCP处理器
	if s.tcpHandler != nil {
		s.tcpHandler.Stop()
	}

	// 停止UDP处理器
	if s.udpHandler != nil {
		s.udpHandler.Stop()
	}

	// 等待所有goroutine结束
	s.wg.Wait()
	log.Println("转发服务器已停止")
}
