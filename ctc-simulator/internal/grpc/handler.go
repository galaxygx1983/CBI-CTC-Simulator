// internal/grpc/handler.go
package grpc

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"ctc-simulator/internal/command"
	"ctc-simulator/internal/protocol"

	log "github.com/sirupsen/logrus"
)

// 数据传送帧（需要序号控制）- CTC接收的帧类型
var dataFrameTypes = map[protocol.FrameType]bool{
	protocol.FIR:  true,
	protocol.SDIQ: true,
	protocol.SDI:  true,
	protocol.SDCI: true,
	protocol.BCC:  true,
	protocol.TSQ:  true,
	protocol.TSD:  true,
	protocol.RSR:  true,
	protocol.ACQ:  true,
}

// 通信控制帧（不需要序号控制）
var controlFrameTypes = map[protocol.FrameType]bool{
	protocol.DC2:    true,
	protocol.DC3:    true,
	protocol.ACK:    true,
	protocol.NACK:   true,
	protocol.VERROR: true,
}

// FrameQueue 发送帧队列
type FrameQueue struct {
	frames []*protocol.Frame
	mu     sync.Mutex
}

func NewFrameQueue() *FrameQueue {
	return &FrameQueue{
		frames: make([]*protocol.Frame, 0),
	}
}

func (q *FrameQueue) Push(frame *protocol.Frame) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.frames = append(q.frames, frame)
}

func (q *FrameQueue) Pop() *protocol.Frame {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.frames) == 0 {
		return nil
	}
	frame := q.frames[0]
	q.frames = q.frames[1:]
	return frame
}

func (q *FrameQueue) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.frames) == 0
}

// FrameHandler 帧处理器
type FrameHandler struct {
	server      *Server
	builder     *command.Builder
	bccTicker   *time.Ticker
	rsrTicker   *time.Ticker
	tsqTicker   *time.Ticker
	sdiqTicker  *time.Ticker
	stopChan    chan struct{}
	loopStarted bool
	mu          sync.Mutex
	// 序号控制
	recvSeq byte
	// CTC 本地状态
	stationState  []byte
	cbiRole       byte
	cbiMode       byte
	lastTimestamp uint64
	// 第一条SDI接收时间
	firstSDITime     time.Time
	sdiqTimerStarted bool
	// 发送队列：SDIQ、ACA、BCC 放入队列
	sendQueue *FrameQueue
	// 等待确认机制
	waitingForAck bool
	lastSentFrame *protocol.Frame
	retryCount    int
	retryTimer    *time.Timer
	retryMu       sync.Mutex
	// 心跳机制
	heartbeatTimer  *time.Timer
	heartbeatMu     sync.Mutex
	lastAckSendTime time.Time // 上次 ACK 发送时间
	ackInterval     time.Duration // ACK 最小间隔（500ms）
	ackTimer        *time.Timer
	ackTimerMu      sync.Mutex
	// 首次 RSR 标志
	firstRSRReceived bool
}

// NewFrameHandler 创建帧处理器
func NewFrameHandler(server *Server) *FrameHandler {
	return &FrameHandler{
		server:        server,
		builder:       command.NewBuilder(),
		stopChan:      make(chan struct{}),
		stationState:  make([]byte, 0),
		cbiRole:       0x00,
		cbiMode:       0x00,
		lastTimestamp: 0,
		sendQueue:     NewFrameQueue(),
		ackInterval:   500 * time.Millisecond, // ACK 最小间隔 500ms
	}
}

// StartCTCLoop 启动 CTC 定期发送循环
func (h *FrameHandler) StartCTCLoop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.loopStarted {
		return
	}

	h.bccTicker = time.NewTicker(5 * time.Second)
	h.rsrTicker = time.NewTicker(20 * time.Second)
	h.tsqTicker = time.NewTicker(10 * time.Minute)
	h.sdiqTicker = time.NewTicker(time.Duration(55+rand.Intn(11)) * time.Second)

	go func() {
		for {
			select {
			case <-h.bccTicker.C:
				h.queueBCC()
			case <-h.rsrTicker.C:
				h.sendRSR()
			case <-h.tsqTicker.C:
				h.sendTSQ()
			case <-h.sdiqTicker.C:
				h.queueSDIQ()
			case <-h.stopChan:
				return
			}
		}
	}()
	h.loopStarted = true
	log.Info("CTC loop started: BCC=5s, RSR=20s, TSQ=10m, SDIQ=~1m")
}

// StopCTCLoop 停止循环
func (h *FrameHandler) StopCTCLoop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.bccTicker != nil {
		h.bccTicker.Stop()
	}
	if h.rsrTicker != nil {
		h.rsrTicker.Stop()
	}
	if h.tsqTicker != nil {
		h.tsqTicker.Stop()
	}
	if h.sdiqTicker != nil {
		h.sdiqTicker.Stop()
	}
	if h.loopStarted {
		close(h.stopChan)
		h.loopStarted = false
	}

	h.heartbeatMu.Lock()
	if h.heartbeatTimer != nil {
		h.heartbeatTimer.Stop()
		h.heartbeatTimer = nil
	}
	h.heartbeatMu.Unlock()

	h.retryMu.Lock()
	if h.retryTimer != nil {
		h.retryTimer.Stop()
		h.retryTimer = nil
	}
	h.retryMu.Unlock()

	log.Info("CTC loop stopped")
}

// resetHeartbeatTimer 重置心跳计时器
// 收到帧后调用，500ms 内没有数据帧则发送 ACK 心跳
func (h *FrameHandler) resetHeartbeatTimer() {
	h.heartbeatMu.Lock()
	defer h.heartbeatMu.Unlock()

	if h.heartbeatTimer != nil {
		h.heartbeatTimer.Stop()
	}

	// 只在队列为空时启动心跳计时器（有数据帧要发送则不需要心跳）
	if h.sendQueue.IsEmpty() {
		h.heartbeatTimer = time.AfterFunc(500*time.Millisecond, func() {
			h.sendHeartbeatAck()
		})
		log.Debug("Heartbeat timer reset (500ms)")
	} else {
		log.Debug("Heartbeat timer skipped - send queue not empty")
	}
}

// stopHeartbeatTimer 停止心跳计时器
func (h *FrameHandler) stopHeartbeatTimer() {
	h.heartbeatMu.Lock()
	defer h.heartbeatMu.Unlock()
	if h.heartbeatTimer != nil {
		h.heartbeatTimer.Stop()
		h.heartbeatTimer = nil
	}
}

// sendACK 统一 ACK 发送入口（所有 ACK 发送必须通过此方法）
// 确保 ACK 间隔控制和序号管理集中
func (h *FrameHandler) sendACK(reason string, force bool) error {
	h.ackTimerMu.Lock()
	defer h.ackTimerMu.Unlock()

	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		log.Debug("Stream not available for ACK")
		return fmt.Errorf("stream not available")
	}

	// 检查 ACK 间隔是否满足 500ms（force=true 时跳过检查，用于数据帧响应）
	if !force && !h.lastAckSendTime.IsZero() {
		elapsed := time.Since(h.lastAckSendTime)
		if elapsed < h.ackInterval {
			// 间隔不足 500ms，延迟发送
			remaining := h.ackInterval - elapsed
			log.Infof("ACK interval too short (%v), delaying for %v (%s)", elapsed, remaining, reason)
			if h.ackTimer != nil {
				h.ackTimer.Stop()
			}
			h.ackTimer = time.AfterFunc(remaining, func() {
				h.sendACK("delayed-"+reason, force)
			})
			return nil
		}
	}

	// 发送 ACK 前停止心跳计时器，避免重复发送
	h.stopHeartbeatTimer()

	// 在发送时获取最新的 ackSeq
	ack := h.builder.BuildACK()
	log.Infof("Sending ACK (%s): seq=%d, ack=%d", reason, ack.SendSeq, ack.AckSeq)

	if err := stream.QueueSend(ack); err != nil {
		log.Warnf("Failed to send ACK: %v", err)
		return err
	}

	h.lastAckSendTime = time.Now()

	// 发送 ACK 后：
	// - 如果发送队列不为空且不等待 ACK → 发送数据帧（捎带确认）
	// - 否则 → 启动心跳计时器
	h.retryMu.Lock()
	canSend := !h.waitingForAck
	h.retryMu.Unlock()

	if canSend && h.trySendNextFrame() {
		// 成功发送数据帧，不再需要心跳
	} else {
		// 队列为空或无法发送，启动心跳计时器
		h.resetHeartbeatTimer()
	}

	return nil
}

// sendHeartbeatAck 发送心跳 ACK（通过统一入口）
// 使用 force=true 跳过 500ms 间隔检查，确保心跳立即发送
func (h *FrameHandler) sendHeartbeatAck() {
	log.Info("sendHeartbeatAck: about to send heartbeat ACK")
	h.sendACK("heartbeat", true)
	log.Info("sendHeartbeatAck: resetting heartbeat timer")
	h.resetHeartbeatTimer()
}

// cancelAckTimer 取消 ACK 延迟计时器（发送数据帧前调用）
func (h *FrameHandler) cancelAckTimer() {
	h.ackTimerMu.Lock()
	defer h.ackTimerMu.Unlock()
	if h.ackTimer != nil {
		h.ackTimer.Stop()
		h.ackTimer = nil
	}
	log.Debug("ACK timer cancelled for data frame")
}

// queueBCC 将BCC加入发送队列
func (h *FrameHandler) queueBCC() {
	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		return
	}

	deviceType := byte(rand.Intn(2) + 1)
	var deviceIndex uint16
	var command byte

	if deviceType == 0x01 {
		deviceIndex = uint16(rand.Intn(100) + 1)
		command = byte(rand.Intn(2) + 1)
	} else {
		deviceIndex = uint16(rand.Intn(200) + 1)
		command = byte(rand.Intn(2))
	}

	bcc := h.builder.BuildBCC(deviceIndex, command)
	log.Infof("Queueing BCC: deviceIndex=%d, command=0x%02X", deviceIndex, command)
	h.queueFrame(bcc)
}

// sendRSR 发送 CTC 状态报告（直接发送，不走队列）
func (h *FrameHandler) sendRSR() {
	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		return
	}

	rsr := h.builder.BuildRSR(true)
	log.Infof("Sending RSR: CTC role=0x55 (Master), mode=0x55 (Auto Control)")
	h.sendFrameImmediate(rsr)
}

// sendTSQ 发送时间同步请求（直接发送，不走队列）
func (h *FrameHandler) sendTSQ() {
	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		return
	}

	tsq := h.builder.BuildTSQ()
	log.Info("Sending TSQ frame")
	h.sendFrameImmediate(tsq)
}

// queueSDIQ 将SDIQ加入发送队列
func (h *FrameHandler) queueSDIQ() {
	h.mu.Lock()
	firstSDITime := h.firstSDITime
	sdiqTimerStarted := h.sdiqTimerStarted
	h.mu.Unlock()

	if firstSDITime.IsZero() {
		log.Debug("SDIQ: No SDI received yet, skipping")
		return
	}

	if time.Since(firstSDITime) < 2*time.Minute {
		log.Debugf("SDIQ: Waiting for 2 minutes since first SDI")
		return
	}

	if !sdiqTimerStarted {
		h.mu.Lock()
		h.sdiqTimerStarted = true
		h.mu.Unlock()
	}

	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		return
	}

	sdiq := h.builder.BuildSDIQ()
	log.Info("Queueing SDIQ frame")
	h.queueFrame(sdiq)

	h.mu.Lock()
	if h.sdiqTicker != nil {
		h.sdiqTicker.Reset(time.Duration(55+rand.Intn(11)) * time.Second)
	}
	h.mu.Unlock()
}

// queueFrame 将帧加入发送队列并尝试发送
func (h *FrameHandler) queueFrame(frame *protocol.Frame) {
	h.sendQueue.Push(frame)
	log.Infof("Queued %s frame", frame.Type)
	h.trySendNextFrame()
}

// trySendNextFrame 尝试发送队列中的下一帧
// 返回 true 表示发送了帧，false 表示队列为空
func (h *FrameHandler) trySendNextFrame() bool {
	h.retryMu.Lock()
	if h.waitingForAck {
		h.retryMu.Unlock()
		return false
	}
	h.retryMu.Unlock()

	frame := h.sendQueue.Pop()
	if frame == nil {
		// 队列为空，不启动心跳计时器
		// 心跳计时器应该只在需要时启动（例如长时间无通信）
		return false
	}

	log.Infof("Sending from queue: %s (seq=%d)", frame.Type, frame.SendSeq)
	h.sendFrameImmediate(frame)
	return true
}

// sendFrameImmediate 立即发送帧并等待确认
// 如果是数据帧，取消待发送的 ACK 计时器
func (h *FrameHandler) sendFrameImmediate(frame *protocol.Frame) error {
	// 连接帧和响应帧不需要重传机制
	// 包括: DC2, DC3, ACK, NACK, VERROR
	if frame.Type == protocol.DC2 || frame.Type == protocol.DC3 ||
		frame.Type == protocol.ACK || frame.Type == protocol.NACK ||
		frame.Type == protocol.VERROR {
		stream := h.server.GetStream()
		if stream == nil || !stream.IsRunning() {
			return ErrNotConnected
		}
		log.Infof("Sending response frame %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)
		return stream.QueueSend(frame)
	}

	h.retryMu.Lock()
	if h.waitingForAck {
		h.retryMu.Unlock()
		log.Warnf("Cannot send %s: waiting for ACK", frame.Type)
		return ErrWaitingForAck
	}
	h.retryMu.Unlock()

	h.stopHeartbeatTimer()

	// 如果是数据帧（非 ACK），取消待发送的 ACK 计时器
	h.cancelAckTimer()

	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		return ErrNotConnected
	}

	log.Infof("Sending data frame %s with piggybacked ack (seq=%d, ack=%d)",
		frame.Type, frame.SendSeq, frame.AckSeq)

	if err := stream.QueueSend(frame); err != nil {
		return err
	}

	h.startFrameRetry(frame)
	return nil
}

// startFrameRetry 启动帧重传计时器
func (h *FrameHandler) startFrameRetry(frame *protocol.Frame) {
	h.retryMu.Lock()
	defer h.retryMu.Unlock()

	if h.retryTimer != nil {
		h.retryTimer.Stop()
	}

	h.lastSentFrame = frame
	h.waitingForAck = true
	h.retryCount = 0

	h.retryTimer = time.AfterFunc(500*time.Millisecond, func() {
		h.handleFrameRetryTimeout()
	})

	log.Debugf("Frame retry timer started for seq=%d", frame.SendSeq)
}

// handleFrameRetryTimeout 处理帧重传超时
func (h *FrameHandler) handleFrameRetryTimeout() {
	h.retryMu.Lock()

	if h.lastSentFrame == nil {
		h.retryMu.Unlock()
		return
	}

	if h.retryCount >= 2 {
		frame := h.lastSentFrame
		h.lastSentFrame = nil
		h.waitingForAck = false
		h.retryCount = 0
		h.retryTimer = nil
		h.retryMu.Unlock()
		log.Errorf("Frame retry failed 3 times (seq=%d), reconnecting...", frame.SendSeq)
		h.sendDC2Reconnect()
		return
	}

	h.retryCount++
	frame := h.lastSentFrame
	h.retryMu.Unlock()

	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		log.Warn("Stream not available for frame retry")
		return
	}

	log.Warnf("Retrying frame %s (seq=%d), attempt %d/2", frame.Type, frame.SendSeq, h.retryCount)
	if err := stream.QueueSend(frame); err != nil {
		log.Warnf("Failed to retry frame: %v", err)
		return
	}

	h.retryMu.Lock()
	h.retryTimer = time.AfterFunc(500*time.Millisecond, func() {
		h.handleFrameRetryTimeout()
	})
	h.retryMu.Unlock()
}

// HandleFrame 处理接收到的帧
func (h *FrameHandler) HandleFrame(frame *protocol.Frame) error {
	log.Infof("Handling frame: %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)

	// DC2 连接帧不受序号控制
	if frame.Type == protocol.DC2 {
		log.Info("Connection frame DC2 received, skipping sequence check")
		h.sendDC2Reconnect()
		return nil
	}

	// DC3 连接帧不受序号控制
	if frame.Type == protocol.DC3 {
		log.Info("Connection frame DC3 received, skipping sequence check")
		h.mu.Lock()
		h.recvSeq = 0
		h.mu.Unlock()
		return h.handleDC3(frame)
	}

	// 控制帧处理（ACK, NACK, VERROR）不受序号控制
	if controlFrameTypes[frame.Type] {
		return h.handleControlFrame(frame)
	}

	// 数据帧序号检查
	if !h.checkDataFrameSequence(frame) {
		return nil // 已处理（重复帧或通信中断）
	}

	// 数据帧处理
	h.builder.UpdateAckSeq(frame.SendSeq)

	switch frame.Type {
	case protocol.RSR:
		return h.handleRSR(frame)
	case protocol.SDI:
		return h.handleSDI(frame)
	case protocol.SDIQ:
		return h.handleSDIQ(frame)
	case protocol.SDCI:
		return h.handleSDCI(frame)
	case protocol.ACQ:
		return h.handleACQ(frame)
	case protocol.BCC:
		return h.handleBCC(frame)
	case protocol.TSD:
		return h.handleTSD(frame)
	case protocol.TSQ:
		return h.handleTSQ(frame)
	case protocol.FIR:
		return h.handleFIR(frame)
	default:
		log.Warnf("Unhandled frame type: %s", frame.Type)
	}

	return nil
}

// handleControlFrame 处理控制帧（ACK, NACK, VERROR）
func (h *FrameHandler) handleControlFrame(frame *protocol.Frame) error {
	switch frame.Type {
	case protocol.ACK:
		return h.handleACK(frame)
	case protocol.NACK:
		return h.handleNACK(frame)
	case protocol.VERROR:
		log.Error("Received VERROR, disconnecting")
		h.StopCTCLoop()
		return nil
	}
	return nil
}

// checkDataFrameSequence 数据帧序号检查
// 规则6: 将帧中的确认序号与自己的发送序号变量比较，两值相等时认为收到了前次发送帧的确认信息，之后将自己的发送序号变量加1
// 规则7: 将帧中的发送序号与自己的接收确认序号变量进行比较，发送序号比接收确认序号大1时，将接收确认序号变量加1
// 规则8: 如果帧中的发送序号等于自己的接收确认序号，则认为发送方发送了重复的数据帧，仍向发送方发送确认信息但不再对接收到的数据进行处理
// 规则9: 如果帧中的发送序号比自己的接收确认序号大于或等于2，则认为通信中断，CTC发送DC2重新初始化通信
func (h *FrameHandler) checkDataFrameSequence(frame *protocol.Frame) bool {
	// 规则6: 检查确认序号
	// 将帧中的确认序号与自己的发送序号变量比较，两值相等时认为收到了前次发送帧的确认信息
	// 之后将自己的发送序号变量加1
	if h.builder.ConfirmAck(frame.AckSeq) {
		log.Debugf("ACK confirmed: ackSeq=%d matched sendSeq, incremented", frame.AckSeq)
	}

	// 规则7/8/9: 检查发送序号
	h.mu.Lock()
	recvSeq := h.recvSeq
	h.mu.Unlock()

	seqDiff := int(frame.SendSeq) - int(recvSeq)
	if seqDiff < 0 {
		seqDiff += 256 // 处理序号回绕
	}

	switch {
	case seqDiff == 1:
		// 规则7: 发送序号比接收确认序号大1，正常帧
		// 接收确认序号变量加1
		h.mu.Lock()
		h.recvSeq = frame.SendSeq
		h.mu.Unlock()
		log.Debugf("Normal frame: sendSeq=%d, recvSeq now=%d", frame.SendSeq, h.recvSeq)
		return true

	case seqDiff == 0:
		// 规则8: 发送序号等于接收确认序号，重复帧
		// 仍向发送方发送确认信息，但不再对接收到的数据进行处理
		log.Warnf("Duplicate frame: %s (seq=%d, recvSeq=%d)", frame.Type, frame.SendSeq, recvSeq)
		h.sendAckForDuplicate(frame.SendSeq)
		return false

	default:
		// 规则9: 发送序号比接收确认序号大于或等于2，通信中断
		// CTC发送DC2重新初始化通信
		log.Errorf("Communication interrupted: expected seq=%d, got seq=%d", recvSeq+1, frame.SendSeq)
		h.sendDC2Reconnect()
		return false
	}
}

// handleACK 处理ACK帧
func (h *FrameHandler) handleACK(frame *protocol.Frame) error {
	// 检查是否确认了之前发送的数据帧
	h.retryMu.Lock()
	if h.lastSentFrame != nil && frame.AckSeq == h.lastSentFrame.SendSeq {
		log.Infof("Frame confirmed by ackSeq=%d, clearing retry state", frame.AckSeq)
		if h.retryTimer != nil {
			h.retryTimer.Stop()
			h.retryTimer = nil
		}
		h.lastSentFrame = nil
		h.waitingForAck = false
		h.retryCount = 0
	}
	h.retryMu.Unlock()

	// 收到 ACK 后，停止心跳计时器，避免重复发送
	h.stopHeartbeatTimer()

	log.Debug("Received ACK frame")

	// 收到 ACK 后，尝试发送队列中的帧
	h.retryMu.Lock()
	canSend := !h.waitingForAck
	h.retryMu.Unlock()

	if canSend && h.trySendNextFrame() {
		// 成功发送数据帧，等待确认期间不需要心跳
	} else {
		// 队列为空或等待确认，启动心跳计时器
		h.resetHeartbeatTimer()
	}

	return nil
}

// handleNACK 处理NACK帧
func (h *FrameHandler) handleNACK(frame *protocol.Frame) error {
	log.Warn("Received NACK")
	// TODO: 重发上一帧
	return nil
}

// handleDC3 处理连接确认
func (h *FrameHandler) handleDC3(frame *protocol.Frame) error {
	log.Infof("=== Received DC3 (seq=%d, ack=%d), connection confirmed ===", frame.SendSeq, frame.AckSeq)

	// DC3 收到后，初始化序号（发送序号=1，接收确认序号=0）
	h.builder.UpdateSendSeq(1)
	h.builder.UpdateAckSeq(0)

	// DC3 收到后，立即发送 RSR
	stream := h.server.GetStream()
	if stream == nil {
		log.Warn("Stream not initialized, cannot send RSR")
		return nil
	}

	rsr := h.builder.BuildRSR(true)
	log.Infof("Sending RSR (seq=%d)", rsr.SendSeq)
	if err := stream.QueueSend(rsr); err != nil {
		return err
	}

	// 启动 CTC 循环
	if !h.loopStarted {
		h.StartCTCLoop()
	}

	return nil
}

// handleRSR 处理 CBI 发送的状态报告
func (h *FrameHandler) handleRSR(frame *protocol.Frame) error {
	log.Info("Received RSR from CBI")

	if len(frame.Data) >= 2 {
		h.mu.Lock()
		h.cbiRole = frame.Data[0]
		h.cbiMode = frame.Data[1]
		h.mu.Unlock()

		roleStr := "Unknown"
		if h.cbiRole == 0x55 {
			roleStr = "Master"
		} else if h.cbiRole == 0xAA {
			roleStr = "Slave"
		}

		modeStr := "Unknown"
		if h.cbiMode == 0x55 {
			modeStr = "Auto Control"
		} else if h.cbiMode == 0xAA {
			modeStr = "Manual Control"
		}

		log.Infof("CBI RSR received: role=0x%02X (%s), mode=0x%02X (%s)", h.cbiRole, roleStr, h.cbiMode, modeStr)
	}

	// 首次 RSR：立即回复 ACK
	if !h.firstRSRReceived {
		h.firstRSRReceived = true
		log.Info("First RSR received, sending ACK immediately")
		// 更新确认序号为收到的帧序号
		h.builder.UpdateAckSeq(frame.SendSeq)
		h.sendACK("rsr-first", true)
		if !h.loopStarted {
			h.StartCTCLoop()
		}
		return nil
	}

	// 后续 RSR：概率响应（70% ACK / 29% BCC / 1% SDIQ）
	randVal := rand.Intn(100)
	stream2 := h.server.GetStream()
	if stream2 == nil {
		return nil
	}

	switch {
	case randVal < 70:
		h.sendACK("rsr-random", true)
	case randVal < 99:
		// BCC 需要随机设备索引和命令
		bcc := h.builder.BuildBCC(uint16(rand.Intn(256)), byte(rand.Intn(2)))
		h.sendFrameImmediate(bcc)
	default:
		sdiq := h.builder.BuildSDIQ()
		h.sendFrameImmediate(sdiq)
	}

	return nil
}

// handleSDIQ 处理站场数据请求
func (h *FrameHandler) handleSDIQ(frame *protocol.Frame) error {
	log.Info("Received SDIQ from CBI, sending SDI")

	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		log.Warn("Stream not available for SDI response")
		return nil
	}

	// 构建SDI响应（捎带确认）
	sdiData := h.stationState
	if len(sdiData) == 0 {
		log.Warn("No station state available, sending empty SDI")
		sdiData = []byte{}
	}

	sdi := h.builder.BuildSDI(sdiData)
	log.Infof("Sending SDI response (seq=%d, len=%d)", sdi.SendSeq, len(sdiData))
	h.sendFrameImmediate(sdi)

	return nil
}

// handleSDI 处理完整站场数据
func (h *FrameHandler) handleSDI(frame *protocol.Frame) error {
	log.Infof("Received SDI from CBI, length=%d", frame.DataLength)

	h.mu.Lock()
	if h.firstSDITime.IsZero() {
		h.firstSDITime = time.Now()
		log.Infof("First SDI received at %v, SDIQ timer will start in 2 minutes", h.firstSDITime)
	}
	h.stationState = make([]byte, len(frame.Data))
	copy(h.stationState, frame.Data)
	h.mu.Unlock()

	log.Infof("Updated station state: total bytes=%d", len(h.stationState))

	// 收到 SDI 后，立即回复（捎带确认或ACK）
	// 有数据帧则捎带确认，否则立即发ACK
	if !h.trySendNextFrame() {
		h.sendImmediateAck()
	}

	return nil
}

// handleSDCI 处理增量站场数据
func (h *FrameHandler) handleSDCI(frame *protocol.Frame) error {
	log.Infof("Received SDCI, length=%d", frame.DataLength)

	// 收到 SDCI 后，立即回复（捎带确认或ACK）
	// 有数据帧则捎带确认，否则立即发ACK
	if !h.trySendNextFrame() {
		h.sendImmediateAck()
	}

	return nil
}

// handleFIR 处理故障信息报告
func (h *FrameHandler) handleFIR(frame *protocol.Frame) error {
	log.Infof("Received FIR, length=%d", frame.DataLength)

	// 收到 FIR 后，立即回复（捎带确认或ACK）
	// 有数据帧则捎带确认，否则立即发ACK
	if !h.trySendNextFrame() {
		h.sendImmediateAck()
	}

	return nil
}

// handleTSD 处理时间同步数据
func (h *FrameHandler) handleTSD(frame *protocol.Frame) error {
	if len(frame.Data) >= 8 {
		timestamp := uint64(frame.Data[0]) |
			uint64(frame.Data[1])<<8 |
			uint64(frame.Data[2])<<16 |
			uint64(frame.Data[3])<<24 |
			uint64(frame.Data[4])<<32 |
			uint64(frame.Data[5])<<40 |
			uint64(frame.Data[6])<<48 |
			uint64(frame.Data[7])<<56
		log.Infof("Received TSD, timestamp=%d", timestamp)

		h.mu.Lock()
		h.lastTimestamp = timestamp
		h.mu.Unlock()
	}

	// 收到 TSD 后，立即回复（捎带确认或ACK）
	if !h.trySendNextFrame() {
		h.sendImmediateAck()
	}

	return nil
}

// handleTSQ 处理时间同步请求（立刻回复TSD）
func (h *FrameHandler) handleTSQ(frame *protocol.Frame) error {
	log.Info("Received TSQ from CBI, immediately replying with TSD")

	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		log.Warn("Stream not available for TSD response")
		return nil
	}

	// 立刻回复 TSD
	tsd := h.builder.BuildTSD(uint64(time.Now().UnixMilli()))
	log.Infof("Sending TSD response (seq=%d)", tsd.SendSeq)

	h.sendFrameImmediate(tsd)

	return nil
}

// handleACQ 处理自律控制请求
func (h *FrameHandler) handleACQ(frame *protocol.Frame) error {
	log.Info("Received ACQ, sending ACA response immediately")

	// 立即发送 ACA（不再延迟 2 秒）
	aca := h.builder.BuildACA(true)
	log.Infof("Sending ACA (agreed) with ackSeq=%d", aca.AckSeq)
	h.sendFrameImmediate(aca)

	return nil
}

// handleBCC 处理控制命令
func (h *FrameHandler) handleBCC(frame *protocol.Frame) error {
	log.Infof("Received BCC: %X", frame.Data)

	// 收到 BCC 后，立即回复（捎带确认或ACK）
	// 有数据帧则捎带确认，否则立即发ACK
	if !h.trySendNextFrame() {
		h.sendImmediateAck()
	}

	return nil
}

// sendImmediateAck 立即发送ACK确认
func (h *FrameHandler) sendImmediateAck() {
	stream := h.server.GetStream()
	if stream == nil || !stream.IsRunning() {
		log.Warn("Stream not available for immediate ACK")
		return
	}

	// 立即 ACK 不检查间隔，因为是对数据帧的响应
	h.sendACK("immediate", true)
}

// GetBuilder 获取命令构建器
func (h *FrameHandler) GetBuilder() *command.Builder {
	return h.builder
}

// sendAckForDuplicate 对重复帧发送 ACK 确认（通过统一入口）
func (h *FrameHandler) sendAckForDuplicate(recvSeq byte) {
	h.builder.UpdateAckSeq(recvSeq)
	h.sendACK("duplicate", true)
	h.resetHeartbeatTimer()
}

// sendDC2Reconnect 发送 DC2 重新建立连接
func (h *FrameHandler) sendDC2Reconnect() {
	h.mu.Lock()
	h.stationState = make([]byte, 0)
	h.cbiRole = 0x00
	h.cbiMode = 0x00
	h.recvSeq = 0
	h.builder.UpdateSendSeq(0)
	h.builder.UpdateAckSeq(0)
	h.mu.Unlock()

	h.retryMu.Lock()
	if h.retryTimer != nil {
		h.retryTimer.Stop()
	}
	h.lastSentFrame = nil
	h.waitingForAck = false
	h.retryCount = 0
	h.retryMu.Unlock()

	h.stopHeartbeatTimer()

	dc2 := &protocol.Frame{
		HeaderLen: 0x04,
		Version:   0x11,
		Type:    protocol.DC2,
		SendSeq: 0x00,
		AckSeq:  0x00,
	}

	stream := h.server.GetStream()
	if stream == nil {
		log.Warn("Stream not initialized, cannot send DC2")
		return
	}

	if err := stream.QueueSend(dc2); err != nil {
		log.Errorf("Failed to send DC2 reconnect: %v", err)
	} else {
		log.Info("Sent DC2 to reconnect after communication interruption")
		h.resetHeartbeatTimer()
	}
}
