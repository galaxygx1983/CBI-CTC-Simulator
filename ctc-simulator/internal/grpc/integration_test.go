//go:build integration
// +build integration

package grpc

import (
	"context"
	"testing"
	"time"

	"ctc-simulator/internal/protocol"
)

// TestIntegrationCTCConnect 测试CTC服务端与CBI客户端连接
// 需要: 启动ctc-simulator服务端
func TestIntegrationCTCConnect(t *testing.T) {
	// 创建服务端
	server := NewServer(":50099")

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Server start failed: %v", err)
	}
	defer server.Stop()

	// 等待服务启动
	time.Sleep(100 * time.Millisecond)

	// 验证服务运行
	if !server.IsRunning() {
		t.Error("Server should be running")
	}
}

// TestIntegrationFrameExchange 测试帧交换
func TestIntegrationFrameExchange(t *testing.T) {
	server := NewServer(":50100")
	ctx := context.Background()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Server start failed: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	// 模拟发送DC2帧
	dc2 := &protocol.Frame{Type: protocol.DC2}
	err := server.GetStream().QueueSend(dc2)
	if err == ErrNotConnected {
		t.Log("Stream not connected (expected in this test)")
	}
}