// internal/grpc/frame_handler_test.go
package grpc

import (
	"testing"

	"cbi-simulator/internal/protocol"
)

func TestNewFrameHandler(t *testing.T) {
	handler := NewFrameHandler(nil)
	if handler == nil {
		t.Fatal("NewFrameHandler returned nil")
	}
}

func TestSendFrame_NilClient(t *testing.T) {
	handler := NewFrameHandler(nil)

	// client 为 nil，应该返回错误而不是 panic
	err := handler.SendDataFrame(protocol.RSR, func() []byte {
		return []byte{0x55, 0x55}
	})

	if err == nil {
		t.Error("Expected error when client is nil")
	}
}

func TestSendDC3(t *testing.T) {
	// SendDC3 需要有效的 client，这里只测试初始化逻辑
	// 新的 FrameHandler 初始化时 sendSeq=0, ackSeq=0
	handler := NewFrameHandler(nil)
	_ = handler // 避免未使用警告
	t.Logf("SendDC3 requires valid client, skipping send test")
}

func TestGetSendSeq(t *testing.T) {
	handler := NewFrameHandler(nil)

	seq := handler.GetSendSeq()
	// 新的 FrameHandler 初始 sendSeq 为 0
	if seq != 0 {
		t.Errorf("Expected initial sendSeq=0, got %d", seq)
	}
}

func TestGetAckSeq(t *testing.T) {
	handler := NewFrameHandler(nil)

	seq := handler.GetAckSeq()
	if seq != 0 {
		t.Errorf("Expected initial ackSeq=0, got %d", seq)
	}
}

func TestGetAckCount(t *testing.T) {
	handler := NewFrameHandler(nil)

	count := handler.GetAckCount()
	if count != 0 {
		t.Errorf("Expected initial ackCount=0, got %d", count)
	}
}

func TestGetRoleState(t *testing.T) {
	handler := NewFrameHandler(nil)

	// 默认为主机
	role := handler.GetRoleState()
	if role != 0x55 {
		t.Errorf("Expected roleState=0x55, got 0x%02X", role)
	}
}

func TestGetControlMode(t *testing.T) {
	handler := NewFrameHandler(nil)

	// 默认为非常站控
	mode := handler.GetControlMode()
	if mode != 0xAA {
		t.Errorf("Expected controlMode=0xAA, got 0x%02X", mode)
	}
}

func TestIsConnected(t *testing.T) {
	handler := NewFrameHandler(nil)

	// 初始未连接
	if handler.IsConnected() {
		t.Error("Expected not connected initially")
	}
}

func TestHandleFrame_VersionMismatch(t *testing.T) {
	handler := NewFrameHandler(nil)

	// 版本号错误
	frame := &protocol.Frame{
		Type:    protocol.ACK,
		Version: 0x10, // 错误版本
		SendSeq: 1,
		AckSeq:  0,
	}

	err := handler.HandleFrame(frame)
	if err == nil {
		t.Error("Expected error for version mismatch")
	}
}