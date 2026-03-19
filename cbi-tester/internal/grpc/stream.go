// internal/grpc/stream.go
package grpc

import (
	"context"
	"errors"
	"sync"

	"cbi-simulator/internal/protocol"

	log "github.com/sirupsen/logrus"
)

const (
	sendQueueSize = 100
	recvQueueSize = 100
)

// streamManager 管理FrameStream的双向通信
type streamManager struct {
	client *Client

	sendQueue chan *protocol.Frame // 发送队列
	recvQueue chan *protocol.Frame // 接收队列

	ctx    context.Context
	cancel context.CancelFunc

	sendWg sync.WaitGroup
	recvWg sync.WaitGroup

	mu      sync.RWMutex
	running bool
}

// newStreamManager 创建流管理器
func newStreamManager(client *Client) *streamManager {
	return &streamManager{
		client:    client,
		sendQueue: make(chan *protocol.Frame, sendQueueSize),
		recvQueue: make(chan *protocol.Frame, recvQueueSize),
	}
}

// Start 启动流管理
func (sm *streamManager) Start(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.running {
		return nil
	}

	sm.ctx, sm.cancel = context.WithCancel(ctx)
	sm.running = true

	// 启动发送协程
	sm.sendWg.Add(1)
	go sm.sendLoop()

	// 启动接收协程
	sm.recvWg.Add(1)
	go sm.recvLoop()

	log.Info("Stream manager started")
	return nil
}

// Stop 停止流管理
func (sm *streamManager) Stop() {
	sm.mu.Lock()
	if !sm.running {
		sm.mu.Unlock()
		return
	}
	sm.running = false
	sm.mu.Unlock()

	if sm.cancel != nil {
		sm.cancel()
	}

	sm.sendWg.Wait()
	sm.recvWg.Wait()

	log.Info("Stream manager stopped")
}

// sendLoop 发送循环
func (sm *streamManager) sendLoop() {
	defer sm.sendWg.Done()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case frame := <-sm.sendQueue:
			if err := sm.client.SendFrame(frame); err != nil {
				log.Errorf("Send frame failed: %v", err)
				if sm.client.onError != nil {
					sm.client.onError(err)
				}
			}
		}
	}
}

// recvLoop 接收循环
func (sm *streamManager) recvLoop() {
	defer sm.recvWg.Done()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case frame := <-sm.recvQueue:
			if sm.client.onFrame != nil {
				sm.client.onFrame(frame)
			}
		}
	}
}

// QueueSend 将帧加入发送队列
func (sm *streamManager) QueueSend(frame *protocol.Frame) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if !sm.running {
		return ErrNotConnected
	}

	select {
	case sm.sendQueue <- frame:
		return nil
	default:
		return ErrSendQueueFull
	}
}

// QueueRecv 将帧加入接收队列
func (sm *streamManager) QueueRecv(frame *protocol.Frame) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if !sm.running {
		return ErrNotConnected
	}

	select {
	case sm.recvQueue <- frame:
		return nil
	default:
		return ErrRecvQueueFull
	}
}

// 错误定义
var (
	ErrNotConnected  = errors.New("not connected")
	ErrSendQueueFull = errors.New("send queue full")
	ErrRecvQueueFull = errors.New("recv queue full")
)