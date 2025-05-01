package utils

import (
	"fmt"
	"io"
	"strings"
)

// IsConnectionClosed 检查错误是否为连接关闭
func IsConnectionClosed(err error) bool {
	return err == io.EOF || strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "connection reset by peer") ||
		strings.Contains(err.Error(), "broken pipe")
}

// ParsePort 将字符串端口转换为整数
func ParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
