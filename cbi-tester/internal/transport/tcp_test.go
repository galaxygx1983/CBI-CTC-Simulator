// internal/transport/tcp_test.go
package transport

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestTCPTransportType(t *testing.T) {
	tcp := NewTCPTransport(8001)
	if tcp.Type() != "tcp" {
		t.Errorf("Type: expected 'tcp', got '%s'", tcp.Type())
	}
}

func TestTCPTransportStartStop(t *testing.T) {
	// 使用随机可用端口
	tcp := NewTCPTransport(0)

	ctx := context.Background()
	err := tcp.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 验证监听
	if !tcp.IsListening() {
		t.Error("TCP should be listening")
	}

	// 停止
	err = tcp.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestTCPTransportConnection(t *testing.T) {
	tcp := NewTCPTransport(0)

	ctx := context.Background()
	err := tcp.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer tcp.Stop()

	// 获取实际端口
	addr := tcp.listener.Addr().String()

	// 模拟客户端连接
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 发送数据
	testData := []byte{0x7D, 0x04, 0x11, 0x00, 0x00, 0x12, 0x49, 0xF7, 0x7E}
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
}