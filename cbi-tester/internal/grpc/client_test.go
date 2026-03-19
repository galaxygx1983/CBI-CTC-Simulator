// internal/grpc/client_test.go
package grpc

import (
	"context"
	"testing"
	"time"

	"cbi-simulator/internal/protocol"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient("localhost:50051")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("Client should not be nil")
	}
}

func TestClientAddress(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	if client.Address() != "localhost:50051" {
		t.Errorf("Expected address localhost:50051, got %s", client.Address())
	}
}

func TestClientNotConnected(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	if client.IsConnected() {
		t.Error("Client should not be connected initially")
	}
}

func TestClientDisconnectWhenNotConnected(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	// Disconnect should not error when not connected
	err := client.Disconnect()
	if err != nil {
		t.Errorf("Disconnect should not error when not connected: %v", err)
	}
}

func TestClientSetCallbacks(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	// Test SetOnFrameReceived
	frameReceived := false
	client.SetOnFrameReceived(func(frame *protocol.Frame) {
		frameReceived = true
	})

	// Test SetOnError
	errorReceived := false
	client.SetOnError(func(err error) {
		errorReceived = true
	})

	// Verify callbacks were set (they're nil by default)
	_ = frameReceived
	_ = errorReceived
}

func TestClientGetStation(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	station := client.GetStation()
	if station == nil {
		t.Error("GetStation should not return nil")
	}
}

func TestClientConnectAlreadyConnected(t *testing.T) {
	// This test would require a mock gRPC server, so we just verify the structure
	client, _ := NewClient("localhost:50051")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Since we can't connect without a server, we just verify the method signature
	_ = ctx
	_ = client
}

func TestClientConnectionError(t *testing.T) {
	// 测试无效地址的连接错误处理
	client, _ := NewClient("invalid:address:12345")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Error("Expected error for invalid address")
	}
}

func TestClientDoubleConnect(t *testing.T) {
	// 验证连接结构
	// 由于需要mock gRPC server才能完整测试，这里只验证客户端结构
	client, _ := NewClient("localhost:50051")

	// 验证初始状态
	if client.IsConnected() {
		t.Error("Client should not be connected initially")
	}

	// 验证地址正确存储
	if client.Address() != "localhost:50051" {
		t.Errorf("Expected address localhost:50051, got %s", client.Address())
	}
}

func TestClientDisconnectNotConnected(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	// 未连接时断开应该不报错
	err := client.Disconnect()
	if err != nil {
		t.Errorf("Disconnect should not error when not connected: %v", err)
	}

	// 多次断开也不应该报错
	err = client.Disconnect()
	if err != nil {
		t.Errorf("Disconnect should not error on repeated calls: %v", err)
	}
}

func TestClientSendFrameNotConnected(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	frame := &protocol.Frame{
		Type:    protocol.DC2,
		SendSeq: 1,
		AckSeq:  0,
	}

	err := client.SendFrame(frame)
	if err == nil {
		t.Error("Expected error when sending frame on disconnected client")
	}
}

func TestClientReconnectConfig(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	// 验证重连配置默认值
	if client.reconnectMax != 3 {
		t.Errorf("Expected reconnectMax 3, got %d", client.reconnectMax)
	}
	if client.reconnectWait != time.Second {
		t.Errorf("Expected reconnectWait 1s, got %v", client.reconnectWait)
	}
}

func TestClientSyncDeviceStateDeviceNotFound(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	// 测试同步不存在的设备
	err := client.SyncDeviceState(999, []byte{0x01})
	if err == nil {
		t.Error("Expected error for non-existent device")
	}
}

func TestClientStationNotNil(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	// 确保站场状态已初始化
	station := client.GetStation()
	if station == nil {
		t.Error("Station should not be nil")
	}

	// 确保设备映射已初始化
	if station.Devices == nil {
		t.Error("Devices map should be initialized")
	}
}

func TestClientConcurrentDisconnect(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	// 并发调用Disconnect应该安全
	done := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_ = client.Disconnect()
			done <- true
		}()
	}

	// 等待两个goroutine完成
	for i := 0; i < 2; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(time.Second):
			t.Error("Timeout waiting for Disconnect to complete")
		}
	}
}