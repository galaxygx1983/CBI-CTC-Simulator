// internal/grpc/server_test.go
package grpc

import (
	"context"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	server := NewServer(":50051")
	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.Address() != ":50051" {
		t.Errorf("Expected address :50051, got %s", server.Address())
	}
}

func TestServerStartStop(t *testing.T) {
	server := NewServer(":51001") // Use higher port to avoid conflicts

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !server.IsRunning() {
		t.Error("Server should be running")
	}

	err = server.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if server.IsRunning() {
		t.Error("Server should not be running")
	}
}