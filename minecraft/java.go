package minecraft

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

// JavaPacket 表示一个 Minecraft Java 版数据包
type JavaPacket struct {
	Length   int32  // 数据包长度
	PacketID byte   // 数据包ID
	Data     []byte // 数据包内容
}

// ParseJavaPacket 解析 Minecraft Java 版数据包
func ParseJavaPacket(data []byte) (*JavaPacket, error) {
	if len(data) < 1 {
		return nil, io.ErrShortBuffer
	}

	reader := bytes.NewReader(data)

	// 读取可变长度的包长度
	length, err := ReadVarInt(reader)
	if err != nil {
		return nil, err
	}

	// 验证长度是否合理
	if length < 0 || length > 2097152 { // 最大允许的包长度 (2MB)
		return nil, fmt.Errorf("invalid packet length: %d", length)
	}

	// 读取包ID
	packetID, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	// 读取数据
	packetData := make([]byte, length-1) // 减1是因为包ID已经读取了1字节
	n, err := io.ReadFull(reader, packetData)
	if err != nil {
		log.Printf("取包数据失败: %v, 应读取 %d 字节，实际读取 %d 字节", err, length-1, n)
		return nil, err
	}

	return &JavaPacket{
		Length:   length,
		PacketID: packetID,
		Data:     packetData,
	}, nil
}

// IsHandshake 判断是否是握手包
func (p *JavaPacket) IsHandshake() bool {
	return p.PacketID == 0x00 // 握手包ID为0x00
}

// IsStatusRequest 判断是否是状态请求包
func (p *JavaPacket) IsStatusRequest() bool {
	return p.PacketID == 0x00 && len(p.Data) == 0 // 状态请求包ID为0x00且没有数据
}

// ReadVarInt 从reader读取变长整数
func ReadVarInt(reader io.Reader) (int32, error) {
	var value int32
	var position int
	var currentByte byte

	for {
		err := binary.Read(reader, binary.BigEndian, &currentByte)
		if err != nil {
			return 0, err
		}

		value |= int32(currentByte&0x7F) << position

		if currentByte&0x80 == 0 {
			break
		}

		position += 7

		if position >= 32 {
			return 0, io.ErrUnexpectedEOF
		}
	}

	return value, nil
}

// WriteVarInt 写入变长整数
func WriteVarInt(value int32) []byte {
	var result bytes.Buffer

	for {
		temp := byte(value & 0x7F)
		value >>= 7

		if value != 0 {
			temp |= 0x80
		}

		result.WriteByte(temp)

		if value == 0 {
			break
		}
	}

	return result.Bytes()
}

// JavaStatusResponse 表示服务器状态响应
type JavaStatusResponse struct {
	Version struct {
		Name     string `json:"name"`     // Minecraft 版本名称
		Protocol int    `json:"protocol"` // 协议版本号
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`    // 最大玩家数
		Online int `json:"online"` // 在线玩家数
		Sample []struct {
			Name string `json:"name"` // 玩家名称
			ID   string `json:"id"`   // 玩家 UUID
		} `json:"sample,omitempty"` // 玩家示例列表
	} `json:"players"`
	Description interface{} `json:"description"`       // 服务器描述/MOTD
	Favicon     string      `json:"favicon,omitempty"` // 服务器图标（Base64编码的PNG）

	// 1.19+ 添加的字段
	EnforcesSecureChat  bool `json:"enforcesSecureChat,omitempty"`  // 是否强制安全聊天
	PreventsChatReports bool `json:"preventsChatReports,omitempty"` // 是否防止聊天举报

	// 支持 ModInfo
	ModInfo *ModInfo `json:"modinfo,omitempty"` // 模组信息
}

// ModInfo 代表 Minecraft 的模组信息
type ModInfo struct {
	Type    string   `json:"type"`              // 模组类型 (forge, fabric 等)
	ModList []string `json:"modList,omitempty"` // 模组列表
}

// PingPacket 代表一个 Minecraft 服务器 ping 请求或响应
type PingPacket struct {
	Time int64 // 时间戳（毫秒）
}

// ModifyJavaStatusResponse 修改状态响应数据，支持完整的 MOTD 特性
func ModifyJavaStatusResponse(data []byte, motd string, maxPlayers, onlinePlayers int) ([]byte, error) {
	// 解析JSON响应
	var response JavaStatusResponse
	err := json.Unmarshal(data, &response)
	if err != nil {
		// 如果解析失败，创建一个新的基本响应
		response = createDefaultStatusResponse()
	}

	// 修改MOTD (支持纯文本、JSON格式或聊天组件格式)
	if motd != "" {
		response.Description = parseMotd(motd)
	}

	// 修改最大玩家数
	if maxPlayers > 0 {
		response.Players.Max = maxPlayers
	}

	// 修改在线玩家数
	if onlinePlayers >= 0 {
		response.Players.Online = onlinePlayers
	}

	// 重新序列化为JSON
	return json.Marshal(response)
}

// parseMotd 解析 MOTD 字符串为适当的格式
func parseMotd(motd string) interface{} {
	// 首先尝试解析为JSON格式，且必须是聊天组件（有 text 字段）
	var jsonMotd map[string]interface{}
	if strings.HasPrefix(motd, "{") && strings.HasSuffix(motd, "}") {
		err := json.Unmarshal([]byte(motd), &jsonMotd)
		if err == nil {
			if _, ok := jsonMotd["text"]; ok {
				return jsonMotd // 是聊天组件格式
			}
		}
	}
	// 不是JSON聊天组件，自动包成{"text": motd}
	return map[string]interface{}{"text": motd}
}

// createDefaultStatusResponse 创建默认的状态响应
func createDefaultStatusResponse() JavaStatusResponse {
	var response JavaStatusResponse

	// 设置版本信息 (使用当前流行的版本作为默认值)
	response.Version.Name = "1.19.3"
	response.Version.Protocol = 761 // 对应1.19.3的协议版本

	// 设置玩家信息
	response.Players.Max = 100
	response.Players.Online = 0

	// 设置默认描述
	response.Description = map[string]string{"text": "A Minecraft Server"}

	return response
}

// GeneratePingResponse 生成ping响应，返回与收到的时间戳相同的数据
func GeneratePingResponse(payload []byte) []byte {
	// 构造完整的数据包
	var fullPacket bytes.Buffer

	// 写入长度
	packetLength := 1 + len(payload) // 1字节包ID + payload长度
	fullPacket.Write(WriteVarInt(int32(packetLength)))

	// 写入包ID - ping响应包ID是 0x01
	fullPacket.WriteByte(0x01)

	// 写入payload (通常是一个8字节的时间戳)
	fullPacket.Write(payload)

	return fullPacket.Bytes()
}

// 常量定义
const (
	// 数据包类型
	JavaStatusRequest byte = 0x00 // 状态请求
	JavaLoginRequest  byte = 0x00 // 登录请求

	// 状态阶段 - 用于标记玩家连接的不同阶段
	HandshakeState byte = 0x00 // 握手阶段
	StatusState    byte = 0x01 // 状态请求阶段
	LoginState     byte = 0x02 // 登录阶段
)

// HandshakePacket 表示握手包数据
type HandshakePacket struct {
	ProtocolVersion int32  // 协议版本
	ServerAddress   string // 服务器地址
	ServerPort      uint16 // 服务器端口
	NextState       byte   // 下一阶段 (1=状态请求, 2=登录)
}

// ParseHandshakePacket 解析握手包
func ParseHandshakePacket(data []byte) (*HandshakePacket, error) {
	if len(data) < 3 { // 至少包含协议版本、空的地址字符串和下一状态
		return nil, io.ErrShortBuffer
	}

	reader := bytes.NewReader(data)

	// 读取协议版本
	protocolVersion, err := ReadVarInt(reader)
	if err != nil {
		return nil, err
	}

	// 读取服务器地址长度
	addrLen, err := ReadVarInt(reader)
	if err != nil {
		return nil, err
	}

	// 读取服务器地址
	addrBytes := make([]byte, addrLen)
	_, err = io.ReadFull(reader, addrBytes)
	if err != nil {
		return nil, err
	}

	// 读取服务器端口
	var serverPort uint16
	err = binary.Read(reader, binary.BigEndian, &serverPort)
	if err != nil {
		return nil, err
	}

	// 读取下一状态
	var nextState byte
	nextState, err = reader.ReadByte()
	if err != nil {
		return nil, err
	}

	return &HandshakePacket{
		ProtocolVersion: protocolVersion,
		ServerAddress:   string(addrBytes),
		ServerPort:      serverPort,
		NextState:       nextState,
	}, nil
}

// GenerateLoginDenyPacket 生成一个登录拒绝包
func GenerateLoginDenyPacket(reason string) []byte {
	// 构造JSON格式的拒绝原因
	reasonJson, _ := json.Marshal(map[string]string{"text": reason})

	// 构建数据包
	var packet bytes.Buffer

	// 写入数据包内容 - 变长数组长度
	packet.Write(WriteVarInt(int32(len(reasonJson))))

	// 写入实际的JSON字符串
	packet.Write(reasonJson)

	// 计算数据包总长度
	packetLen := packet.Len() + 1 // +1 是因为还有一个包ID

	// 构造完整的数据包
	var fullPacket bytes.Buffer

	// 写入长度
	fullPacket.Write(WriteVarInt(int32(packetLen)))

	// 写入包ID - 登录拒绝包ID是 0x00
	fullPacket.WriteByte(0x00)

	// 写入数据包内容
	fullPacket.Write(packet.Bytes())

	return fullPacket.Bytes()
}
