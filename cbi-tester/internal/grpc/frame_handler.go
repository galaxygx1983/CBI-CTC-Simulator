// internal/grpc/frame_handler.go
// CBI客户端帧处理器 - 彻底重构
package grpc

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"cbi-simulator/internal/protocol"

	log "github.com/sirupsen/logrus"
)

// 常量定义
const (
	HeaderLen            = 0x04                   // 首部长度固定为4
	Version              = 0x11                   // 协议版本号
	RoleStateMaster      = 0x55                    // 主机状态
	RoleStateBackup      = 0xAA                    // 备机状态
	ControlModeAuto      = 0x55                    // 自律控制模式
	ControlModeEmergency = 0xAA                    // 非常站控模式
	defaultDelay         = 10 * time.Millisecond // 延时10毫秒
)

// 数据传送帧（需要序号控制）
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
	protocol.ACA:  true,
}

// 通信控制帧（不需要序号控制）
var controlFrameTypes = map[protocol.FrameType]bool{
	protocol.DC2:    true,
	protocol.DC3:    true,
	protocol.ACK:    true,
	protocol.NACK:   true,
	protocol.VERROR: true,
}

// FrameHandler 帧处理器
type FrameHandler struct {
	client *Client
	mu     sync.Mutex

	// 序号管理
	sendSeq byte // 发送序号变量
	ackSeq  byte // 接收确认序号变量

	// 状态变量
	ackCount    int  // ACK计数器
	roleState   byte // 主备状态变量：0x55=主机, 0xAA=备机
	controlMode byte // 控制模式变量：0x55=自律控制, 0xAA=非常站控
	connected   bool // 连接状态

	// VERROR状态：收到VERROR后断开通信，不再响应任何报文（包括DC2）
	verrorReceived bool

	// NACK重传机制
	lastSentFrame        *protocol.Frame // 最近发送的一帧
	nackConsecutiveCount int              // NACK连续计数

	// 下一帧发送时间
	nextFrameSendTime time.Time // 下一帧发送时间，初始值为 1970-01-01 00:00:00
	nextFrameMu       sync.Mutex
	ackTimerDone      chan struct{} // ACK定时器停止信号

	// 延时发送控制
	delayTimer *time.Timer

	// 故障注入支持
	customAckTimeout time.Duration // 自定义 ACK 超时时间（0 表示使用默认值）

	// 回调函数
	onDC2  func(*protocol.Frame)
	onDC3  func(*protocol.Frame)
	onRSR  func(*protocol.Frame)
	onSDI  func(*protocol.Frame)
	onSDCI func(*protocol.Frame)
	onSDIQ func(*protocol.Frame)
	onACA  func(*protocol.Frame)
	onACK  func(*protocol.Frame)
	onTSQ  func(*protocol.Frame)
	onBCC  func(*protocol.Frame)
	onTSD  func(*protocol.Frame)
	onFIR  func(*protocol.Frame)
}

// NewFrameHandler 创建帧处理器
func NewFrameHandler(client *Client) *FrameHandler {
	h := &FrameHandler{
		client:               client,
		roleState:            RoleStateMaster,      // 默认主机
		controlMode:          ControlModeEmergency, // 默认非常站控
		nextFrameSendTime:    time.Time{},          // 初始化为 1970-01-01 00:00:00
		ackTimerDone:         make(chan struct{}),
		verrorReceived:       false,
		nackConsecutiveCount: 0,
	}

	// 启动ACK定时检查goroutine
	go h.ackTimerLoop()

	return h
}

// Stop 停止帧处理器
func (h *FrameHandler) Stop() {
	if h.ackTimerDone != nil {
		close(h.ackTimerDone)
	}
}

// ackTimerLoop ACK定时检查循环
// 每毫秒检查一次，如果当前时间 >= 下一帧发送时间，则发送ACK
func (h *FrameHandler) ackTimerLoop() {
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-h.ackTimerDone:
			log.Debug("ACK timer loop stopped")
			return
		case <-ticker.C:
			h.nextFrameMu.Lock()
			nextTime := h.nextFrameSendTime
			h.nextFrameMu.Unlock()

			if !nextTime.IsZero() && time.Now().After(nextTime) {
				// 清除下一帧发送时间，避免重复发送
				h.nextFrameMu.Lock()
				h.nextFrameSendTime = time.Time{}
				h.nextFrameMu.Unlock()

				// 发送ACK
				if h.connected {
					log.Info("ACK timer triggered, sending ACK")
					if err := h.SendACK(); err != nil {
						log.Warnf("Failed to send timer ACK: %v", err)
					}
				}
			}
		}
	}
}

// updateNextFrameSendTime 更新下一帧发送时间（接收时间 + 490ms 或自定义超时）
func (h *FrameHandler) updateNextFrameSendTime() {
	h.nextFrameMu.Lock()
	defer h.nextFrameMu.Unlock()
	timeout := 490 * time.Millisecond // 默认超时
	if h.customAckTimeout > 0 {
		timeout = h.customAckTimeout
	}
	h.nextFrameSendTime = time.Now().Add(timeout)
}

// SetAckTimeout 设置 ACK 超时时间（用于故障注入）
func (h *FrameHandler) SetAckTimeout(timeout time.Duration) {
	h.nextFrameMu.Lock()
	h.customAckTimeout = timeout
	h.nextFrameMu.Unlock()
}

// === 发送帧方法 ===

// SendDataFrame 发送数据传送帧
// 数据传送帧包括：FIR, SDIQ, SDI, SDCI, BCC, TSQ, TSD, RSR, ACQ, ACA
// 发送后发送序号变量递增
func (h *FrameHandler) SendDataFrame(frameType protocol.FrameType, buildData func() []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查 client 是否为 nil
	if h.client == nil {
		return errors.New("client is nil")
	}

	// 检查是否已收到VERROR，如果是则不再发送
	if h.verrorReceived {
		return errors.New("communication disconnected due to VERROR")
	}

	var data []byte
	if buildData != nil {
		data = buildData()
	}

	// 数据帧：sendSeq填入当前发送序号变量的值，发送后递增
	frame := &protocol.Frame{
		HeaderLen:  HeaderLen,
		Type:        frameType,
		SendSeq:     h.sendSeq,
		AckSeq:      h.ackSeq,
		Version:     Version,
		DataLength:  uint16(len(data)),
		Data:        data,
	}

	log.Infof("SendDataFrame: %s (seq=%d, ack=%d)", frameType, h.sendSeq, h.ackSeq)

	// 保存最近发送的帧（用于NACK重传）
	h.lastSentFrame = frame

	if err := h.client.SendFrame(frame); err != nil {
		return fmt.Errorf("send frame failed: %w", err)
	}

	// 发送成功后，发送序号变量递增
	h.sendSeq++
	if h.sendSeq == 0 {
		h.sendSeq = 1 // 序号0保留，从1开始
	}

	return nil
}

// SendACK 发送ACK帧
// 规则：ACK帧的发送序号固定保持最后一次发送数据传送帧的发送序号
// 规则：ACK帧的确认序号填写最近接收到的正确数据传送帧的发送序号
// 注意：sendSeq变量在发送数据帧后已递增，所以最后发送的数据帧序号是 sendSeq - 1
func (h *FrameHandler) SendACK() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.client == nil {
		return errors.New("client is nil")
	}

	// ACK帧的发送序号 = 最后发送数据帧的序号
	// sendSeq在发送数据帧后已递增，所以最后发送的数据帧序号是 sendSeq - 1
	// 如果sendSeq == 1（初始状态），需要特殊处理：
	//   - sendSeq初始化为1表示还没发送过数据帧
	//   - 此时ACK的sendSeq应该是1（初始序号）
	ackSendSeq := h.sendSeq - 1
	if ackSendSeq == 0 {
		ackSendSeq = 1 // 序号0保留，最小有效序号是1
	}

	// ACK帧的确认序号 = ackSeq（最近正确接收数据帧的发送序号）
	frame := &protocol.Frame{
		HeaderLen: HeaderLen,
		Type:      protocol.ACK,
		SendSeq:   ackSendSeq,  // 最后发送数据帧的序号
		AckSeq:    h.ackSeq,    // 确认序号变量
		Version:   Version,
	}

	log.Infof("SendACK: seq=%d (last data frame seq), ack=%d", frame.SendSeq, frame.AckSeq)

	return h.client.SendFrame(frame)
}

// SendFrameWithDelay 延时发送数据帧
func (h *FrameHandler) SendFrameWithDelay(frameType protocol.FrameType, buildData func() []byte, delay time.Duration) error {
	time.Sleep(delay)
	return h.SendDataFrame(frameType, buildData)
}

// SendDC3 发送DC3（序号固定为0x00）
func (h *FrameHandler) SendDC3() error {
	h.mu.Lock()
	// DC3发送后，发送序号置1，接收确认序号置0
	h.sendSeq = 1
	h.ackSeq = 0
	h.mu.Unlock()

	frame := &protocol.Frame{
		HeaderLen: HeaderLen,
		Type:      protocol.DC3,
		SendSeq:   0x00,
		AckSeq:    0x00,
		Version:   Version,
	}

	log.Info("SendDC3: initialized sendSeq=1, ackSeq=0")

	return h.client.SendFrame(frame)
}

// SendFrame 发送控制帧（不需要序号控制）
// 用于发送 NACK, VERROR 等控制帧
// 控制帧的发送序号保持最后发送数据帧的序号
func (h *FrameHandler) SendFrame(frameType protocol.FrameType) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.client == nil {
		return errors.New("client is nil")
	}

	// 控制帧：发送序号 = 最后发送数据帧的序号
	// sendSeq在发送数据帧后已递增，所以最后发送的数据帧序号是 sendSeq - 1
	controlSendSeq := h.sendSeq - 1
	if controlSendSeq == 0 {
		controlSendSeq = 1 // 序号0保留，最小有效序号是1
	}

	// 控制帧：ackSeq使用ackSeq变量
	frame := &protocol.Frame{
		HeaderLen: HeaderLen,
		Type:      frameType,
		SendSeq:   controlSendSeq, // 最后发送数据帧的序号
		AckSeq:    h.ackSeq,        // 确认序号变量
		Version:   Version,
	}

	log.Infof("SendFrame: %s (seq=%d, ack=%d)", frameType, controlSendSeq, h.ackSeq)

	return h.client.SendFrame(frame)
}

// SendACKWithDelay 延时10ms发送ACK
func (h *FrameHandler) SendACKWithDelay() error {
	time.Sleep(defaultDelay)
	return h.SendACK()
}

// SendNACK 发送NACK
func (h *FrameHandler) SendNACK() error {
	return h.SendFrame(protocol.NACK)
}

// SendVERROR 发送VERROR
func (h *FrameHandler) SendVERROR() error {
	return h.SendFrame(protocol.VERROR)
}

// === 接收帧处理 ===

// HandleFrame 处理接收到的帧
func (h *FrameHandler) HandleFrame(frame *protocol.Frame) error {
	log.Infof("Handling frame: %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)

	// 检查是否已收到VERROR（通信已断开）
	h.mu.Lock()
	verror := h.verrorReceived
	h.mu.Unlock()

	if verror {
		// 已收到VERROR，不再处理任何报文（包括DC2）
		log.Warnf("Received frame %s but communication disconnected due to VERROR, ignoring", frame.Type)
		return errors.New("communication disconnected due to VERROR")
	}

	// 更新下一帧发送时间（接收时间 + 490ms）
	h.updateNextFrameSendTime()

	// 1. 版本号检查
	if frame.Version != Version {
		log.Warnf("Version mismatch: got 0x%02X, expect 0x%02X", frame.Version, Version)
		h.SendVERROR()
		h.Disconnect()
		return errors.New("version mismatch")
	}

	// 2. DC2特殊处理（不受序号控制）
	if frame.Type == protocol.DC2 {
		return h.handleDC2(frame)
	}

	// 3. DC3特殊处理（不受序号控制）
	if frame.Type == protocol.DC3 {
		return h.handleDC3(frame)
	}

	// 4. 控制帧处理（不受序号控制）
	if controlFrameTypes[frame.Type] {
		return h.handleControlFrame(frame)
	}

	// 5. 数据帧序号检查
	if !h.checkDataFrameSequence(frame) {
		return nil // 已处理（重复帧或通信中断）
	}

	// 6. 根据帧类型分发处理
	switch frame.Type {
	case protocol.RSR:
		return h.handleRSR(frame)
	case protocol.SDI:
		return h.handleSDI(frame)
	case protocol.SDIQ:
		return h.handleSDIQ(frame)
	case protocol.SDCI:
		return h.handleSDCI(frame)
	case protocol.ACA:
		return h.handleACA(frame)
	case protocol.BCC:
		return h.handleBCC(frame)
	case protocol.TSD:
		return h.handleTSD(frame)
	case protocol.TSQ:
		return h.handleTSQ(frame)
	case protocol.FIR:
		return h.handleFIR(frame)
	case protocol.ACQ:
		return h.handleACQ(frame)
	default:
		log.Warnf("Unhandled frame type: %s", frame.Type)
	}

	return nil
}

// handleDC2 处理DC2
// 将ACK计数器置为0，主备状态置为主机，控制模式置为非常站控
// 延时10ms回复DC3，发送序号置1，接收确认序号置0
// 如果已收到VERROR，不再响应
func (h *FrameHandler) handleDC2(frame *protocol.Frame) error {
	log.Info("Received DC2, initializing state")

	h.mu.Lock()
	// 检查是否已收到VERROR
	if h.verrorReceived {
		h.mu.Unlock()
		log.Warn("Received DC2 but communication disconnected due to VERROR, ignoring")
		return errors.New("communication disconnected due to VERROR")
	}

	// 重置NACK连续计数
	h.nackConsecutiveCount = 0
	// 重置状态
	h.ackCount = 0
	h.roleState = RoleStateMaster
	h.controlMode = ControlModeEmergency
	h.sendSeq = 1
	h.ackSeq = 0
	h.connected = true
	h.mu.Unlock()

	// 触发回调
	if h.onDC2 != nil {
		h.onDC2(frame)
	}

	return nil
}

// handleDC3 处理DC3
func (h *FrameHandler) handleDC3(frame *protocol.Frame) error {
	h.mu.Lock()
	h.sendSeq = 1
	h.ackSeq = 0
	h.connected = true
	h.mu.Unlock()

	if h.onDC3 != nil {
		h.onDC3(frame)
	}
	return nil
}

// handleControlFrame 处理控制帧（ACK, NACK, VERROR）
func (h *FrameHandler) handleControlFrame(frame *protocol.Frame) error {
	switch frame.Type {
	case protocol.ACK:
		// 收到ACK，重置NACK连续计数
		h.mu.Lock()
		h.nackConsecutiveCount = 0
		h.mu.Unlock()
		return h.handleACK(frame)
	case protocol.NACK:
		return h.handleNACK(frame)
	case protocol.VERROR:
		return h.handleVERROR(frame)
	}
	return nil
}

// checkDataFrameSequence 数据帧序号检查
// 规则7: 将帧中的发送序号与自己的接收确认序号变量进行比较，发送序号比接收确认序号大1时，将接收确认序号变量加1
// 规则8: 如果帧中的发送序号等于自己的接收确认序号，则认为发送方发送了重复的数据帧，仍向发送方发送确认信息但不再对接收到的数据进行处理
// 规则9: 如果帧中的发送序号比自己的接收确认序号大于或等于2，则认为通信中断，CBI等待CTC重新初始化通信
// 注意：sendSeq只在发送数据帧后递增，不在接收帧时递增
func (h *FrameHandler) checkDataFrameSequence(frame *protocol.Frame) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 规则7/8/9: 检查发送序号
	// 将帧中的发送序号与自己的接收确认序号变量进行比较
	seqDiff := int(frame.SendSeq) - int(h.ackSeq)
	if seqDiff < 0 {
		seqDiff += 256 // 处理序号回绕
	}

	switch {
	case seqDiff == 1:
		// 规则7: 发送序号比接收确认序号大1，正常帧
		// 接收确认序号变量加1
		h.ackSeq = frame.SendSeq
		// 重置NACK连续计数（收到正常数据帧）
		h.nackConsecutiveCount = 0
		log.Debugf("Normal frame: sendSeq=%d, ackSeq now=%d", frame.SendSeq, h.ackSeq)
		return true

	case seqDiff == 0:
		// 规则8: 发送序号等于接收确认序号，重复帧
		// 仍向发送方发送确认信息，但不再对接收到的数据进行处理
		// 重置NACK连续计数
		h.nackConsecutiveCount = 0
		log.Warnf("Duplicate frame: %s (seq=%d, ackSeq=%d)", frame.Type, frame.SendSeq, h.ackSeq)
		go h.SendACK()
		return false

	default:
		// 规则9: 发送序号比接收确认序号大于或等于2，通信中断
		// CBI不发任何数据给CTC，等待CTC重新初始化通信
		log.Errorf("Communication interrupted: expected seq=%d, got seq=%d", h.ackSeq+1, frame.SendSeq)
		h.connected = false
		return false
	}
}

// handleRSR 处理RSR
func (h *FrameHandler) handleRSR(frame *protocol.Frame) error {
	if h.onRSR != nil {
		h.onRSR(frame)
	}
	return nil
}

// handleSDI 处理SDI
func (h *FrameHandler) handleSDI(frame *protocol.Frame) error {
	if h.onSDI != nil {
		h.onSDI(frame)
	}
	return nil
}

// handleSDIQ 处理SDIQ
func (h *FrameHandler) handleSDIQ(frame *protocol.Frame) error {
	if h.onSDIQ != nil {
		h.onSDIQ(frame)
	}
	return nil
}

// handleSDCI 处理SDCI
func (h *FrameHandler) handleSDCI(frame *protocol.Frame) error {
	if h.onSDCI != nil {
		h.onSDCI(frame)
	}
	return nil
}

// handleACA 处理ACA
func (h *FrameHandler) handleACA(frame *protocol.Frame) error {
	if len(frame.Data) > 0 && frame.Data[0] == ControlModeAuto {
		h.mu.Lock()
		h.controlMode = ControlModeAuto
		h.mu.Unlock()
		log.Info("Control mode changed to auto")
	}

	// 延时10ms回复ACK
	go func() {
		time.Sleep(defaultDelay)
		h.SendACK()
	}()

	if h.onACA != nil {
		h.onACA(frame)
	}
	return nil
}

// handleBCC 处理BCC
func (h *FrameHandler) handleBCC(frame *protocol.Frame) error {
	// 延时10ms回复ACK
	go func() {
		time.Sleep(defaultDelay)
		h.SendACK()
	}()

	if h.onBCC != nil {
		h.onBCC(frame)
	}
	return nil
}

// handleTSD 处理TSD
func (h *FrameHandler) handleTSD(frame *protocol.Frame) error {
	// 延时10ms回复ACK
	go func() {
		time.Sleep(defaultDelay)
		h.SendACK()
	}()

	if h.onTSD != nil {
		h.onTSD(frame)
	}
	return nil
}

// handleTSQ 处理TSQ
func (h *FrameHandler) handleTSQ(frame *protocol.Frame) error {
	if h.onTSQ != nil {
		h.onTSQ(frame)
	}
	return nil
}

// handleFIR 处理FIR
func (h *FrameHandler) handleFIR(frame *protocol.Frame) error {
	// 延时10ms回复ACK
	go func() {
		time.Sleep(defaultDelay)
		h.SendACK()
	}()

	if h.onFIR != nil {
		h.onFIR(frame)
	}
	return nil
}

// handleACQ 处理ACQ
func (h *FrameHandler) handleACQ(frame *protocol.Frame) error {
	// 回复ACA（同意自律控制）
	// TODO: 根据实际需求处理
	return nil
}

// handleACK 处理ACK
func (h *FrameHandler) handleACK(frame *protocol.Frame) error {
	h.mu.Lock()
	h.ackCount++
	count := h.ackCount
	h.mu.Unlock()

	log.Debugf("ACK count: %d", count)

	// 触发ACK回调
	if h.onACK != nil {
		h.onACK(frame)
	}

	return nil
}

// handleNACK 处理NACK
// 规则：NACK连续计数，如果小于5则重发最近发送的帧，如果大于等于5则断开连接
func (h *FrameHandler) handleNACK(frame *protocol.Frame) error {
	log.Warn("Received NACK")

	h.mu.Lock()
	// NACK连续计数加1
	h.nackConsecutiveCount++
	count := h.nackConsecutiveCount
	lastFrame := h.lastSentFrame
	h.mu.Unlock()

	log.Infof("NACK consecutive count: %d", count)

	// 如果NACK连续计数大于等于5，断开通信
	if count >= 5 {
		log.Error("NACK consecutive count >= 5, disconnecting communication")
		h.mu.Lock()
		h.verrorReceived = true
		h.connected = false
		h.mu.Unlock()
		return errors.New("NACK consecutive count >= 5, communication disconnected")
	}

	// 如果小于5，延时10ms重发最近发送的帧
	if lastFrame != nil {
		log.Infof("Resending last frame: %s (seq=%d) after 10ms", lastFrame.Type, lastFrame.SendSeq)
		go func() {
			time.Sleep(10 * time.Millisecond)
			h.mu.Lock()
			if h.verrorReceived || !h.connected {
				h.mu.Unlock()
				return
			}
			h.mu.Unlock()

			if h.client != nil {
				if err := h.client.SendFrame(lastFrame); err != nil {
					log.Warnf("Failed to resend frame: %v", err)
				} else {
					log.Infof("Resent frame: %s (seq=%d)", lastFrame.Type, lastFrame.SendSeq)
				}
			}
		}()
	} else {
		log.Warn("No last sent frame to resend")
	}

	return nil
}

// handleVERROR 处理VERROR
// 收到VERROR后断开通信，不再响应任何报文（包括DC2）
func (h *FrameHandler) handleVERROR(frame *protocol.Frame) error {
	log.Error("Received VERROR, disconnecting communication permanently")

	h.mu.Lock()
	h.verrorReceived = true
	h.connected = false
	h.ackCount = 0
	h.nackConsecutiveCount = 0
	h.mu.Unlock()

	return errors.New("received VERROR, communication permanently disconnected")
}

// Disconnect 断开连接
func (h *FrameHandler) Disconnect() {
	h.mu.Lock()
	h.connected = false
	h.ackCount = 0
	h.mu.Unlock()
}

// === 状态获取方法 ===

// GetSendSeq 获取发送序号
func (h *FrameHandler) GetSendSeq() byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sendSeq
}

// GetAckSeq 获取接收确认序号
func (h *FrameHandler) GetAckSeq() byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ackSeq
}

// GetAckCount 获取ACK计数器
func (h *FrameHandler) GetAckCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ackCount
}

// GetRoleState 获取主备状态
func (h *FrameHandler) GetRoleState() byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.roleState
}

// GetControlMode 获取控制模式
func (h *FrameHandler) GetControlMode() byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.controlMode
}

// IsConnected 是否已连接
func (h *FrameHandler) IsConnected() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.connected
}

// === 回调设置方法 ===

func (h *FrameHandler) OnDC2(callback func(*protocol.Frame)) {
	h.onDC2 = callback
}

func (h *FrameHandler) OnDC3(callback func(*protocol.Frame)) {
	h.onDC3 = callback
}

func (h *FrameHandler) OnRSR(callback func(*protocol.Frame)) {
	h.onRSR = callback
}

func (h *FrameHandler) OnSDI(callback func(*protocol.Frame)) {
	h.onSDI = callback
}

func (h *FrameHandler) OnSDCI(callback func(*protocol.Frame)) {
	h.onSDCI = callback
}

func (h *FrameHandler) OnSDIQ(callback func(*protocol.Frame)) {
	h.onSDIQ = callback
}

func (h *FrameHandler) OnACA(callback func(*protocol.Frame)) {
	h.onACA = callback
}

func (h *FrameHandler) OnACK(callback func(*protocol.Frame)) {
	h.onACK = callback
}

func (h *FrameHandler) OnTSQ(callback func(*protocol.Frame)) {
	h.onTSQ = callback
}

func (h *FrameHandler) OnBCC(callback func(*protocol.Frame)) {
	h.onBCC = callback
}

func (h *FrameHandler) OnTSD(callback func(*protocol.Frame)) {
	h.onTSD = callback
}

func (h *FrameHandler) OnFIR(callback func(*protocol.Frame)) {
	h.onFIR = callback
}