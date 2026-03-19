// internal/grpc/handler_test.go
package grpc

import (
	"testing"

	"ctc-simulator/internal/protocol"
)

func TestNewFrameHandler(t *testing.T) {
	server := NewServer(":50055")
	handler := NewFrameHandler(server)

	if handler == nil {
		t.Fatal("NewFrameHandler returned nil")
	}
}

func TestHandleDC2(t *testing.T) {
	server := NewServer(":50056")
	handler := NewFrameHandler(server)

	frame := &protocol.Frame{Type: protocol.DC2}
	err := handler.HandleFrame(frame)

	if err != nil {
		t.Errorf("HandleFrame DC2 failed: %v", err)
	}
}