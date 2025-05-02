package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// PlayerConfig 定义玩家相关配置
type PlayerConfig struct {
	Max    int `yaml:"max"`    // 最大玩家数
	Online int `yaml:"online"` // 在线玩家数
}

// Config 定义转发器配置
type Config struct {
	TCPAddr              string        `yaml:"tcp_addr"`                // TCP本地监听地址
	UDPAddr              string        `yaml:"udp_addr"`                // UDP本地监听地址
	RemoteTCPAddr        string        `yaml:"remote_tcp_addr"`         // TCP远程转发地址
	RemoteUDPAddr        string        `yaml:"remote_udp_addr"`         // UDP远程转发地址
	Protocol             string        `yaml:"protocol"`                // 协议（tcp 或 udp）
	UseProxyProto        bool          `yaml:"use_proxy_proto"`         // 是否使用 Proxy Protocol
	ProxyProtoVer        int           `yaml:"proxy_proto_ver"`         // Proxy Protocol 版本 (1 或 2)
	DisableUDPProxyProto bool          `yaml:"disable_udp_proxy_proto"` // 为 UDP 连接禁用 Proxy Protocol
	Timeout              time.Duration `yaml:"timeout"`                 // 连接超时时间
	BufferSize           int           `yaml:"buffer_size"`             // 缓冲区大小
	LogConnections       bool          `yaml:"log_connections"`         // 是否记录连接信息

	// 后端服务器不可用时的 MOTD 配置
	FallbackMode        bool         `yaml:"fallback_mode"`         // 启用后端不可用时的应急模式
	FallbackMotd        string       `yaml:"fallback_motd"`         // 后端不可用时显示的 MOTD
	FallbackKickMessage string       `yaml:"fallback_kick_message"` // 玩家尝试进入时的踢出信息
	Version             string       `yaml:"version"`               // Minecraft 版本号（如 1.20.4）
	ProtocolVersion     string       `yaml:"protocol_version"`      // 协议号（如 765）
	Players             PlayerConfig `yaml:"players"`               // 玩家数量配置
}

// NewDefaultConfig 返回默认配置
func NewDefaultConfig() Config {
	return Config{
		TCPAddr:              "[::]:25565", // 默认监听所有IPv6和IPv4地址（双栈）
		UDPAddr:              "[::]:19132", // 默认监听所有IPv6和IPv4地址（双栈）
		RemoteTCPAddr:        "127.0.0.1:25566",
		RemoteUDPAddr:        "127.0.0.1:19133",
		Protocol:             "tcp",
		UseProxyProto:        false,
		ProxyProtoVer:        2,
		DisableUDPProxyProto: false,
		Timeout:              5 * time.Minute,
		BufferSize:           32 * 1024,
		LogConnections:       true,

		// 后端不可用时的默认配置
		FallbackMode:        true,
		FallbackMotd:        "§cABMC-Forwarder 服务器维护中...",
		FallbackKickMessage: "§c服务器正在维护，请稍后再试！",
		Version:             "1.20.4",
		ProtocolVersion:     "765",
		Players: PlayerConfig{
			Max:    100,
			Online: 0,
		},
	}
}

// LoadFromFile 从YAML文件加载配置
func LoadFromFile(filePath string) (*Config, error) {
	// 读取配置文件
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析YAML
	config := NewDefaultConfig() // 使用默认值作为基础
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("解析YAML配置失败: %v", err)
	}

	return &config, nil
}

// SaveToFile 保存配置到YAML文件
func (c *Config) SaveToFile(filePath string) error {
	// 将配置转换为YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("转换配置为YAML失败: %v", err)
	}

	// 写入文件
	err = ioutil.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// CreateDefaultConfigFile 创建默认配置文件
func CreateDefaultConfigFile(filePath string) error {
	// 检查文件是否已存在
	_, err := os.Stat(filePath)
	if err == nil {
		return fmt.Errorf("配置文件已存在: %s", filePath)
	}

	// 创建默认配置
	config := NewDefaultConfig()

	// 保存到文件
	return config.SaveToFile(filePath)
}
