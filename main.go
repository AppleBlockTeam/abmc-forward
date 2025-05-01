package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/AppleBlockTeam/abmc-forward/config"
	"github.com/AppleBlockTeam/abmc-forward/server"
)

var (
	configPath   = flag.String("config", "config.yaml", "配置文件路径")
	createConfig = flag.Bool("create-config", false, "创建默认配置文件")
)

func main() {
	// 解析命令行参数
	flag.Parse()

	log.Printf("Hi! ABMC-Forward")

	// 解析配置文件的绝对路径
	absConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		log.Fatalf("解析配置文件路径失败: %v\n", err)
	}

	// 如果指定了创建配置文件
	if *createConfig {
		err := config.CreateDefaultConfigFile(absConfigPath)
		if err != nil {
			log.Fatalf("创建配置文件失败: %v\n", err)
		}
		log.Printf("已创建默认配置文件: %s\n", absConfigPath)
		return
	}

	// 加载配置
	cfg, err := config.LoadFromFile(absConfigPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v\n", err)
	}

	// 创建服务器实例
	srv := server.New(*cfg)

	// 设置全局服务器实例，用于健康检查器和处理器之间的通信
	server.SetGlobalServer(srv)

	// 启动服务器
	if err := srv.Start(); err != nil {
		log.Fatalf("启动服务器失败: %v\n", err)
	}

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 停止服务器
	srv.Stop()
}
