package proxy

import (
	"bytes"
	"fmt"
	"net"

	"github.com/AppleBlockTeam/abmc-forward/utils"
	"github.com/pires/go-proxyproto"
)

// WriteProxyProtocolHeader 向连接写入代理协议头
func WriteProxyProtocolHeader(clientAddr, serverAddr net.Addr, version int, conn net.Conn) error {
	clientIP, clientPort, _ := net.SplitHostPort(clientAddr.String())
	serverIP, serverPort, _ := net.SplitHostPort(serverAddr.String())

	// 解析客户端和服务器IP
	clientIPAddr := net.ParseIP(clientIP)
	serverIPAddr := net.ParseIP(serverIP)

	// 判断IP类型并选择对应的传输协议
	var transportProtocol proxyproto.AddressFamilyAndProtocol
	if clientIPAddr.To4() != nil && serverIPAddr.To4() != nil {
		// IPv4
		transportProtocol = proxyproto.TCPv4
	} else {
		// IPv6
		transportProtocol = proxyproto.TCPv6
	}

	header := &proxyproto.Header{
		Version:           byte(version),
		Command:           proxyproto.PROXY,
		TransportProtocol: transportProtocol,
		SourceAddr: &net.TCPAddr{
			IP:   clientIPAddr,
			Port: utils.ParsePort(clientPort),
		},
		DestinationAddr: &net.TCPAddr{
			IP:   serverIPAddr,
			Port: utils.ParsePort(serverPort),
		},
	}

	// 写入 Proxy Protocol 头
	_, err := header.WriteTo(conn)
	return err
}

// WriteProxyProtocolUDP 为UDP连接生成代理协议头数据
func WriteProxyProtocolUDP(clientAddr, serverAddr net.Addr, version int) ([]byte, error) {
	clientIP, clientPort, _ := net.SplitHostPort(clientAddr.String())
	serverIP, serverPort, _ := net.SplitHostPort(serverAddr.String())

	// 解析客户端和服务器IP
	clientIPAddr := net.ParseIP(clientIP)
	serverIPAddr := net.ParseIP(serverIP)

	// 判断IP类型并选择对应的传输协议
	var transportProtocol proxyproto.AddressFamilyAndProtocol
	if clientIPAddr.To4() != nil && serverIPAddr.To4() != nil {
		// IPv4
		transportProtocol = proxyproto.UDPv4
	} else {
		// IPv6
		transportProtocol = proxyproto.UDPv6
	}

	// 创建 PROXY Protocol 头部
	header := &proxyproto.Header{
		Version:           byte(version),
		Command:           proxyproto.PROXY,
		TransportProtocol: transportProtocol,
		SourceAddr: &net.UDPAddr{
			IP:   clientIPAddr,
			Port: utils.ParsePort(clientPort),
		},
		DestinationAddr: &net.UDPAddr{
			IP:   serverIPAddr,
			Port: utils.ParsePort(serverPort),
		},
	}

	// 使用字节缓冲区来正确获取二进制数据
	buffer := new(bytes.Buffer)
	_, err := header.WriteTo(buffer)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// GenerateProxyProtocolV1Header 生成 PROXY Protocol V1 头部（文本格式）
func GenerateProxyProtocolV1Header(clientAddr, serverAddr net.Addr) ([]byte, error) {
	clientIP, clientPort, _ := net.SplitHostPort(clientAddr.String())
	serverIP, serverPort, _ := net.SplitHostPort(serverAddr.String())

	// 判断 IP 类型
	clientIPAddr := net.ParseIP(clientIP)
	ipFamily := "TCP4"
	if clientIPAddr.To4() == nil {
		ipFamily = "TCP6" // 即使是 UDP，PROXY v1 仍然使用 TCPx 标识
	}

	// PROXY v1 格式: "PROXY {协议} {源IP} {目的IP} {源端口} {目的端口}\r\n"
	header := fmt.Sprintf("PROXY %s %s %s %s %s\r\n",
		ipFamily, clientIP, serverIP, clientPort, serverPort)

	return []byte(header), nil
}

// WrapListener 使用代理协议包装监听器
func WrapListener(listener net.Listener, version int) net.Listener {
	proxyListener := &proxyproto.Listener{
		Listener: listener,
		Policy: func(upstream net.Addr) (proxyproto.Policy, error) {
			// 我们不需要验证客户端是否使用了PROXY协议
			// 因为在这个应用场景中，代理服务器向后端发送PROXY协议信息，而不是接收
			return proxyproto.USE, nil
		},
	}
	return proxyListener
}
