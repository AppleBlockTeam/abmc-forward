package minecraft

import (
	"bytes"
	"encoding/binary"
)

// BedrockPacket 表示一个 Minecraft 基岩版数据包
type BedrockPacket struct {
	PacketID byte   // 数据包ID
	Data     []byte // 数据包内容
}

// 基岩版包ID常量
const (
	UnconnectedPongID      byte = 0x1C // 未连接状态响应包
	OpenConnectionRequest1 byte = 0x05 // 开放连接请求1
	OpenConnectionReply1   byte = 0x06 // 开放连接应答1
	OpenConnectionRequest2 byte = 0x07 // 开放连接请求2
	OpenConnectionReply2   byte = 0x08 // 开放连接应答2
	DisconnectNotification byte = 0x15 // 断开连接通知
)

// RakNet魔数 - 用于识别RakNet协议
var RakNetMagic = []byte{0x00, 0xFF, 0xFF, 0x00, 0xFE, 0xFE, 0xFE, 0xFE, 0xFD, 0xFD, 0xFD, 0xFD, 0x12, 0x34, 0x56, 0x78}

// ParseBedrockPacket 解析基岩版数据包
func ParseBedrockPacket(data []byte) *BedrockPacket {
	if len(data) < 1 {
		return nil
	}

	return &BedrockPacket{
		PacketID: data[0],
		Data:     data[1:],
	}
}

// IsUnconnectedPong 判断是否是未连接的状态响应包
func (p *BedrockPacket) IsUnconnectedPong() bool {
	return p.PacketID == UnconnectedPongID
}

// ModifyBedrockMotd 修改基岩版状态数据
// 基岩版的状态数据格式为:
// [Magic(16字节)][ServerGUID(8字节)][服务器信息字符串长度(2字节)][服务器信息字符串]
func ModifyBedrockMotd(data []byte, motd string, maxPlayers, onlinePlayers int) []byte {
	if len(data) < 35 { // 至少包含魔数(16) + ServerGUID(8) + 长度(2) + 最小MOTD长度(至少9)
		return data
	}

	// 检查是否是UnconnectedPong包
	if data[0] != UnconnectedPongID {
		return data
	}

	// 找到MOTD字符串开始的位置
	// MOTD数据开始于 1(包ID) + 16(Magic) + 8(ServerGUID) + 2(字符串长度) = 27
	pos := 27

	// 读取原始MOTD字符串长度
	strLen := binary.BigEndian.Uint16(data[pos-2 : pos])

	// 解析原始MOTD字符串
	// 基岩版MOTD格式类似于:
	// MCPE;服务器名称;协议版本;Minecraft版本;当前玩家数;最大玩家数;服务器唯一ID;地图名称;游戏模式;
	oldMotdBytes := data[pos : pos+int(strLen)]
	oldMotdParts := bytes.Split(oldMotdBytes, []byte{';'})

	// 确保有足够的部分
	if len(oldMotdParts) < 9 {
		return data
	}

	// 创建新的MOTD
	newMotdParts := make([][]byte, len(oldMotdParts))
	copy(newMotdParts, oldMotdParts)

	// 修改服务器名称
	if motd != "" {
		newMotdParts[1] = []byte(motd)
	}

	// 修改玩家数量信息
	if onlinePlayers >= 0 {
		newMotdParts[4] = []byte(string(itoa(onlinePlayers)))
	}

	if maxPlayers > 0 {
		newMotdParts[5] = []byte(string(itoa(maxPlayers)))
	}

	// 组合新的MOTD字符串
	newMotdBytes := bytes.Join(newMotdParts, []byte{';'})

	// 创建新的数据包
	result := make([]byte, pos+len(newMotdBytes))
	copy(result, data[:pos-2]) // 复制包ID, Magic 和 ServerGUID

	// 写入新的字符串长度
	binary.BigEndian.PutUint16(result[pos-2:pos], uint16(len(newMotdBytes)))

	// 复制新的MOTD字符串
	copy(result[pos:], newMotdBytes)

	return result
}

// GenerateDisconnectPacket 生成一个基岩版断开连接的数据包
func GenerateDisconnectPacket(reason string) []byte {
	// 基岩版不支持断开连接时发送自定义原因
	// 我们生成一个标准的断开通知包
	packet := make([]byte, 17) // 1字节包ID + 16字节魔数

	// 设置包ID
	packet[0] = DisconnectNotification

	// 复制魔数
	copy(packet[1:], RakNetMagic)

	return packet
}

// GenerateCustomPongPacket 生成一个自定义的状态响应包
// 这个包用于在服务器不可用时显示自定义的MOTD信息
func GenerateCustomPongPacket(motd string, maxPlayers int, onlinePlayers int) []byte {
	// 构造响应包
	// [PacketID(1)][Magic(16)][ServerGUID(8)][Timestamp(8)][ServerID Length(2)][ServerID]
	const guidValue int64 = 0x12345678ABCDEF // 使用固定的GUID值
	timestamp := int64(0)                    // 使用0作为时间戳

	// 构造MOTD字符串
	// 基岩版MOTD格式: MCPE;服务器名称;协议版本;游戏版本;在线玩家数;最大玩家数;服务器唯一ID;地图名称;游戏模式;
	motdStr := []byte("MCPE;")
	motdStr = append(motdStr, []byte(motd)...)
	motdStr = append(motdStr, []byte(";527;1.19.50;")...)
	motdStr = append(motdStr, []byte(itoa(onlinePlayers))...)
	motdStr = append(motdStr, ';')
	motdStr = append(motdStr, []byte(itoa(maxPlayers))...)
	motdStr = append(motdStr, []byte(";13253860892328930865;Bedrock level;Survival;1;19132;19133;")...)

	// 计算响应大小: 1(包ID) + 16(魔数) + 8(GUID) + 8(时间戳) + 2(字符串长度) + len(motdStr)
	packetSize := 1 + 16 + 8 + 8 + 2 + len(motdStr)
	packet := make([]byte, packetSize)

	// 设置包ID
	packet[0] = UnconnectedPongID

	// 复制魔数
	copy(packet[1:17], RakNetMagic)

	// 设置服务器GUID (8字节)
	binary.BigEndian.PutUint64(packet[17:25], uint64(guidValue))

	// 设置时间戳 (8字节)
	binary.BigEndian.PutUint64(packet[25:33], uint64(timestamp))

	// 设置服务器ID字符串长度 (2字节)
	binary.BigEndian.PutUint16(packet[33:35], uint16(len(motdStr)))

	// 复制服务器ID字符串
	copy(packet[35:], motdStr)

	return packet
}

// itoa 是一个简单的整数到字符串的转换函数
func itoa(n int) []byte {
	if n == 0 {
		return []byte{'0'}
	}

	// 计算所需位数
	length := 0
	for temp := n; temp > 0; temp /= 10 {
		length++
	}

	// 生成字符串
	result := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		result[i] = byte('0' + n%10)
		n /= 10
	}

	return result
}
