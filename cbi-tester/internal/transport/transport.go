// internal/transport/transport.go
package transport

import (
	"context"
	"io"
)

// Transport 传输接口
type Transport interface {
	// Type 返回传输类型
	Type() string

	// Start 启动传输服务
	Start(ctx context.Context) error

	// Stop 停止传输服务
	Stop() error

	// Send 发送数据
	Send(data []byte) error

	// Receive 接收数据
	Receive() ([]byte, error)

	// SetOnDataHandler 设置数据接收回调
	SetOnDataHandler(handler func(data []byte))

	// IsConnected 返回连接状态
	IsConnected() bool
}

// TransportType 传输类型
type TransportType string

const (
	TransportTCP    TransportType = "tcp"
	TransportSerial TransportType = "serial"
	TransportPipe   TransportType = "pipe"
)

// Connection 连接信息
type Connection struct {
	ID       string
	Remote   string
	Conn     io.ReadWriteCloser
	DataType string // "client" or "server"
}