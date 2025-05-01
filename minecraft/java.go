package minecraft

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
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

	// 读取包ID
	packetID, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	// 读取数据
	packetData := make([]byte, length-1) // 减1是因为包ID已经读取了1字节
	_, err = io.ReadFull(reader, packetData)
	if err != nil {
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
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"sample"`
	} `json:"players"`
	Description map[string]interface{} `json:"description"`
	Favicon     string                 `json:"favicon,omitempty"`
	ModInfo     interface{}            `json:"modinfo,omitempty"`
}

// ModifyJavaStatusResponse 修改状态响应数据
func ModifyJavaStatusResponse(data []byte, motd string, maxPlayers, onlinePlayers int) ([]byte, error) {
	// 解析JSON响应
	var response JavaStatusResponse
	err := json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	// 修改MOTD (支持纯文本或JSON格式)
	if motd != "" {
		// 首先尝试解析为JSON格式
		var jsonMotd map[string]interface{}
		if strings.HasPrefix(motd, "{") && strings.HasSuffix(motd, "}") {
			err = json.Unmarshal([]byte(motd), &jsonMotd)
			if err == nil {
				// 成功解析为JSON
				response.Description = jsonMotd
			} else {
				// JSON解析失败，使用纯文本格式
				response.Description = map[string]interface{}{"text": motd}
			}
		} else {
			// 不是JSON格式，直接使用纯文本
			response.Description = map[string]interface{}{"text": motd}
		}
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
