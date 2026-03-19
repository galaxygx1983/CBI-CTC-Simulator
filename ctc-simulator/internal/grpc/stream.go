// internal/grpc/stream.go
package grpc

import (
	"context"
	"errors"
	"sync"
	"time"

	pb "ctc-simulator/internal/pb/cbi/v1"
	"ctc-simulator/internal/protocol"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	sendQueueSize = 100
	recvQueueSize = 100
)

var (
	ErrNotConnected  = errors.New("not connected")
	ErrSendQueueFull = errors.New("send queue full")
	ErrRecvQueueFull = errors.New("receive queue full")
	ErrWaitingForAck = errors.New("waiting for ACK of previous frame")
)

// StreamManager 管理FrameStream的双向通信
type StreamManager struct {
	server *Server

	sendQueue chan *protocol.Frame
	recvQueue chan *protocol.Frame

	activeStream grpc.BidiStreamingServer[pb.FrameRequest, pb.FrameResponse]

	ctx    context.Context
	cancel context.CancelFunc

	mu           sync.RWMutex
	running      bool
	lastSendTime time.Time  // 上次发送时间
	sendMu       sync.Mutex // 发送互斥锁
}

// NewStreamManager 创建流管理器
func NewStreamManager(server *Server) *StreamManager {
	return &StreamManager{
		server:    server,
		sendQueue: make(chan *protocol.Frame, sendQueueSize),
		recvQueue: make(chan *protocol.Frame, recvQueueSize),
	}
}

// Start 启动流管理
func (sm *StreamManager) Start(ctx context.Context, stream grpc.BidiStreamingServer[pb.FrameRequest, pb.FrameResponse]) error {
	sm.mu.Lock()

	if sm.running {
		sm.mu.Unlock()
		return nil
	}

	sm.ctx, sm.cancel = context.WithCancel(ctx)
	sm.activeStream = stream
	sm.running = true

	// 启动协程
	go sm.sendLoop()
	go sm.recvLoop()

	// 释放锁后再发送 DC2，避免 QueueSend 阻塞
	sm.mu.Unlock()

	// CBI 连接成功后，立即发送 DC2
	dc2 := &protocol.Frame{
		HeaderLen: 0x04,
		Version:   0x11,
		Type:    protocol.DC2,
		SendSeq: 0x00,
		AckSeq:  0x00,
	}
	log.Info("CBI connected, sending DC2")
	log.Infof("DC2 frame created: type=%s, sendSeq=%d, ackSeq=%d", dc2.Type, dc2.SendSeq, dc2.AckSeq)

	if err := sm.QueueSend(dc2); err != nil {
		log.Errorf("Failed to send DC2: %v", err)
	} else {
		log.Info("DC2 queued successfully, waiting for sendLoop to send")
	}

	log.Info("Stream manager started")
	return nil
}

// Stop 停止流管理
func (sm *StreamManager) Stop() {
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

	log.Info("Stream manager stopped")
}

// sendLoop 发送循环
func (sm *StreamManager) sendLoop() {
	log.Info("sendLoop started")
	for {
		select {
		case <-sm.ctx.Done():
			log.Info("sendLoop: context done, exiting")
			return
		case frame := <-sm.sendQueue:
			log.Infof("sendLoop: received frame from channel: %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)
			sm.mu.RLock()
			stream := sm.activeStream
			sm.mu.RUnlock()

			if stream == nil {
				log.Warn("sendLoop: stream is nil")
				continue
			}

			pbFrame := FrameToProto(frame)
			resp := &pb.FrameResponse{Frame: pbFrame}

			log.Infof("sendLoop: sending frame: %s (type=0x%02X, seq=%d, pbType=%v)", frame.Type, byte(frame.Type), frame.SendSeq, pbFrame.Type)
			if err := stream.Send(resp); err != nil {
				log.Errorf("sendLoop: Send frame failed: %v", err)
			} else {
				log.Infof("sendLoop: Frame sent successfully: %s (pbType=%v)", frame.Type, pbFrame.Type)
			}
		}
	}
}

// recvLoop 接收循环
func (sm *StreamManager) recvLoop() {
	log.Info("recvLoop started")
	for {
		select {
		case <-sm.ctx.Done():
			log.Info("recvLoop: context done, exiting")
			return
		case frame := <-sm.recvQueue:
			// 处理接收到的帧
			if sm.server != nil {
				log.Debugf("Received frame: %s", frame.Type)
			}
		}
	}
}

// QueueSend 将帧加入发送队列
func (sm *StreamManager) QueueSend(frame *protocol.Frame) error {
	sm.mu.RLock()
	running := sm.running
	sm.mu.RUnlock()

	log.Infof("QueueSend: %s (seq=%d, ack=%d), running=%v", frame.Type, frame.SendSeq, frame.AckSeq, running)

	if !running {
		log.Warn("QueueSend: stream manager not running")
		return ErrNotConnected
	}

	select {
	case sm.sendQueue <- frame:
		log.Infof("QueueSend success: %s sent to channel", frame.Type)
		return nil
	default:
		log.Warn("QueueSend failed: queue full")
		return ErrSendQueueFull
	}
}

// QueueRecv 将帧加入接收队列
func (sm *StreamManager) QueueRecv(frame *protocol.Frame) error {
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

// IsRunning 返回运行状态
func (sm *StreamManager) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.running
}
