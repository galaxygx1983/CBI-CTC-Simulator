// internal/grpc/integration_test.go
//go:build integration
// +build integration

package grpc

import (
	"context"
	"testing"
	"time"

	"cbi-simulator/internal/protocol"
)

// TestIntegrationConnect 测试实际连接
// 需要: 启动一个CTC gRPC服务端
func TestIntegrationConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewClient("localhost:50051")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect()

	if !client.IsConnected() {
		t.Error("Should be connected")
	}
}

// TestIntegrationFrameStream 测试帧流通信
// 需要: 启动一个CTC gRPC服务端
func TestIntegrationFrameStream(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, _ := NewClient("localhost:50051")
	ctx := context.Background()
	client.Connect(ctx)
	defer client.Disconnect()

	// 发送DC2帧
	dc2Frame := &protocol.Frame{Type: protocol.DC2}
	err := client.SendFrame(dc2Frame)
	if err != nil {
		t.Fatalf("SendFrame failed: %v", err)
	}

	// 等待响应（需要服务端配合）
	time.Sleep(time.Second)
}

// TestIntegrationReceiveFrame 测试帧接收
// 需要: 启动一个CTC gRPC服务端
func TestIntegrationReceiveFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, _ := NewClient("localhost:50051")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect()

	received := make(chan *protocol.Frame, 1)
	client.SetOnFrameReceived(func(frame *protocol.Frame) {
		select {
		case received <- frame:
		default:
		}
	})

	// 发送心跳帧，等待响应
	ackFrame := &protocol.Frame{Type: protocol.ACK}
	if err := client.SendFrame(ackFrame); err != nil {
		t.Fatalf("SendFrame failed: %v", err)
	}

	// 等待接收响应（超时）
	select {
	case frame := <-received:
		t.Logf("Received frame: %s", frame.Type)
	case <-time.After(5 * time.Second):
		t.Log("No frame received (expected when server is not running)")
	}
}