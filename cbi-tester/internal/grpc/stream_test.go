// internal/grpc/stream_test.go
package grpc

import (
	"context"
	"testing"
	"time"

	"cbi-simulator/internal/protocol"
)

func TestStreamManager(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	if sm == nil {
		t.Fatal("StreamManager should not be nil")
	}
}

func TestStreamManagerSendQueue(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	// 测试发送队列
	if cap(sm.sendQueue) != 100 {
		t.Errorf("Expected sendQueue capacity 100, got %d", cap(sm.sendQueue))
	}
}

func TestStreamManagerContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	sm.ctx = ctx
	sm.cancel = cancel

	select {
	case <-sm.ctx.Done():
		t.Error("Context should not be done")
	default:
		// OK
	}
}

func TestStreamManagerQueueSendNotRunning(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	frame := &protocol.Frame{
		Type:    protocol.DC2,
		SendSeq: 1,
	}

	err := sm.QueueSend(frame)
	if err != ErrNotConnected {
		t.Errorf("Expected ErrNotConnected, got %v", err)
	}
}

func TestStreamManagerQueueRecvNotRunning(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	frame := &protocol.Frame{
		Type:    protocol.DC2,
		SendSeq: 1,
	}

	err := sm.QueueRecv(frame)
	if err != ErrNotConnected {
		t.Errorf("Expected ErrNotConnected, got %v", err)
	}
}

func TestStreamManagerStartStop(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	ctx := context.Background()

	// 启动
	err := sm.Start(ctx)
	if err != nil {
		t.Errorf("Start should not fail: %v", err)
	}

	// 验证运行状态
	sm.mu.RLock()
	running := sm.running
	sm.mu.RUnlock()

	if !running {
		t.Error("StreamManager should be running after Start")
	}

	// 重复启动应该返回nil（已经运行）
	err = sm.Start(ctx)
	if err != nil {
		t.Errorf("Double start should return nil, got %v", err)
	}

	// 停止
	sm.Stop()

	sm.mu.RLock()
	running = sm.running
	sm.mu.RUnlock()

	if running {
		t.Error("StreamManager should not be running after Stop")
	}

	// 重复停止应该安全
	sm.Stop()
}

func TestStreamManagerRecvQueueCapacity(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	if cap(sm.recvQueue) != 100 {
		t.Errorf("Expected recvQueue capacity 100, got %d", cap(sm.recvQueue))
	}
}

func TestStreamManagerQueueSendRunning(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	ctx := context.Background()
	err := sm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sm.Stop()

	// 发送少量帧应该成功
	for i := 0; i < 10; i++ {
		frame := &protocol.Frame{
			Type:    protocol.DC2,
			SendSeq: byte(i),
		}
		err := sm.QueueSend(frame)
		if err != nil {
			t.Errorf("QueueSend failed: %v", err)
		}
	}
}

func TestStreamManagerQueueRecvRunning(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	ctx := context.Background()
	err := sm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sm.Stop()

	// 接收少量帧应该成功
	for i := 0; i < 10; i++ {
		frame := &protocol.Frame{
			Type:    protocol.DC2,
			SendSeq: byte(i),
		}
		err := sm.QueueRecv(frame)
		if err != nil {
			t.Errorf("QueueRecv failed: %v", err)
		}
	}
}

func TestStreamManagerContextCancellation(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	ctx, cancel := context.WithCancel(context.Background())
	err := sm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 验证正在运行
	sm.mu.RLock()
	running := sm.running
	sm.mu.RUnlock()

	if !running {
		t.Error("StreamManager should be running")
	}

	// 取消上下文
	cancel()

	// 等待goroutine完成
	time.Sleep(50 * time.Millisecond)

	// 停止管理器
	sm.Stop()

	sm.mu.RLock()
	running = sm.running
	sm.mu.RUnlock()

	if running {
		t.Error("StreamManager should not be running after context cancellation")
	}
}

func TestStreamManagerConcurrentQueueSend(t *testing.T) {
	client, _ := NewClient("localhost:50051")
	sm := newStreamManager(client)

	ctx := context.Background()
	err := sm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sm.Stop()

	// 并发发送
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(seq int) {
			frame := &protocol.Frame{
				Type:    protocol.DC2,
				SendSeq: byte(seq),
			}
			_ = sm.QueueSend(frame)
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(time.Second):
			t.Error("Timeout waiting for concurrent QueueSend")
			return
		}
	}
}