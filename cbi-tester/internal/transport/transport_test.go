// internal/transport/transport_test.go
package transport

import (
	"testing"
)

func TestTransportInterface(t *testing.T) {
	// 测试接口方法签名
	var _ Transport = (*TCPTransport)(nil)
}

func TestTransportType(t *testing.T) {
	tcp := NewTCPTransport(8001)
	if tcp.Type() != "tcp" {
		t.Errorf("TCP Transport type: expected 'tcp', got '%s'", tcp.Type())
	}
}