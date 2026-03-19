// internal/simulator/cbi.go
// CBI模拟器 - 报文收发流程重构
package simulator

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"cbi-simulator/internal/config"
	"cbi-simulator/internal/protocol"
	"cbi-simulator/internal/station"
	"cbi-simulator/internal/transport"

	log "github.com/sirupsen/logrus"
)

// 主备状态值常量
const (
	RoleStateMaster = 0x55 // 主机
	RoleStateBackup = 0xAA // 备机
)

// 控制模式常量
const (
	ControlModeAuto      = 0x55 // 自律控制
	ControlModeEmergency = 0xAA // 非常站控
)

// CBISimulator CBI模拟器
type CBISimulator struct {
	config    *config.Config
	transport transport.Transport
	running   bool
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc

	// 站场状态管理
	stationMgr *station.StationStateManager

	// 连接状态
	connected bool

	// 序号管理（关键协议字段）
	sendSeq    byte // 发送序号变量
	recvAckSeq byte // 接收确认序号变量（期望接收的序号）
	seqMu      sync.Mutex

	// 状态变量
	ackCount    int  // ACK计数器
	roleState   byte // 主备状态变量：0x55=主机, 0xAA=备机
	controlMode byte // 控制模式变量：0x55=自律控制, 0xAA=非常站控

	// 回调
	onFrameReceived func(frame *protocol.Frame)
}

// NewCBISimulator 创建CBI模拟器
func NewCBISimulator(cfg *config.Config) *CBISimulator {
	return &CBISimulator{
		config:      cfg,
		roleState:   RoleStateMaster,   // 默认主机
		controlMode: ControlModeEmergency, // 默认非常站控
		ackCount:    0,
		sendSeq:     1,
		recvAckSeq:  0,
	}
}

// Start 启动模拟器
func (s *CBISimulator) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("simulator already running")
	}

	// 加载站场配置
	if err := s.loadStationConfig(); err != nil {
		log.Warnf("Failed to load station config: %v, using default", err)
	}

	// 创建传输层
	switch s.config.Server.Mode {
	case "tcp":
		s.transport = transport.NewTCPTransport(s.config.Server.TCP.Port)
	default:
		return fmt.Errorf("unsupported transport mode: %s", s.config.Server.Mode)
	}

	// 设置数据接收回调
	s.transport.SetOnDataHandler(s.handleData)

	// 启动传输层
	s.ctx, s.cancel = context.WithCancel(ctx)
	if err := s.transport.Start(s.ctx); err != nil {
		return fmt.Errorf("start transport failed: %w", err)
	}

	s.running = true
	s.connected = false
	log.Infof("CBI Simulator started (role: 0x%02X, mode: 0x%02X)", s.roleState, s.controlMode)

	return nil
}

// Stop 停止模拟器
func (s *CBISimulator) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	s.connected = false

	if s.cancel != nil {
		s.cancel()
	}

	if s.transport != nil {
		s.transport.Stop()
	}

	log.Info("CBI Simulator stopped")
	return nil
}

// IsRunning 返回运行状态
func (s *CBISimulator) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// loadStationConfig 加载站场配置
func (s *CBISimulator) loadStationConfig() error {
	// 加载码位表
	codebitTable, err := station.LoadCodebitTable("configs/lgxtq.zl")
	if err != nil {
		return fmt.Errorf("load codebit table: %w", err)
	}

	// 创建站场状态管理器
	s.stationMgr = station.NewStationStateManager(codebitTable)
	log.Infof("Loaded %d devices from codebit table", len(codebitTable.Devices))

	return nil
}

// handleData 处理接收到的数据
func (s *CBISimulator) handleData(data []byte) {
	log.Debugf("Received %d bytes: %X", len(data), data)

	frame, err := protocol.DecodeFrame(data)
	if err != nil {
		log.Errorf("Decode frame failed: %v", err)
		// CRC错误，发送NACK
		s.sendNACK()
		return
	}

	log.Infof("Received frame: %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)

	// 处理帧
	s.handleFrame(frame)
}

// handleFrame 处理帧（核心协议逻辑）
func (s *CBISimulator) handleFrame(frame *protocol.Frame) {
	// 1. 版本号检查
	if frame.Version != protocol.Version {
		log.Warnf("Version mismatch: got 0x%02X, expect 0x%02X", frame.Version, protocol.Version)
		s.sendVERROR()
		s.disconnect()
		return
	}

	// 2. DC2特殊处理（不受序号控制）
	if frame.Type == protocol.DC2 {
		s.handleDC2(frame)
		return
	}

	// 3. 序号检查
	if !s.checkAndUpdateSequence(frame) {
		return
	}

	// 4. 根据帧类型分发处理
	switch frame.Type {
	case protocol.ACK:
		s.handleACK(frame)
	case protocol.RSR:
		s.handleRSR(frame)
	case protocol.ACA:
		s.handleACA(frame)
	case protocol.TSD:
		s.handleTSD(frame)
	case protocol.BCC:
		s.handleBCC(frame)
	case protocol.SDIQ:
		s.handleSDIQ(frame)
	case protocol.NACK:
		s.handleNACK(frame)
	case protocol.VERROR:
		log.Error("Received VERROR, disconnecting")
		s.disconnect()
	default:
		// 其他帧类型
		if s.onFrameReceived != nil {
			s.onFrameReceived(frame)
		}
	}
}

// handleDC2 处理DC2连接请求
// 1. ACK计数器置为0
// 2. 主备状态变量置为主机（0x55）
// 3. 控制模式变量置为非常站控（0xAA）
// 4. 延时10毫秒回复DC3
// 5. 发送序号变量置为1
// 6. 接收确认序号变量置为0
func (s *CBISimulator) handleDC2(frame *protocol.Frame) {
	log.Info("Received DC2, initializing connection")

	// 重置状态
	s.mu.Lock()
	s.ackCount = 0
	s.roleState = RoleStateMaster
	s.controlMode = ControlModeEmergency
	s.mu.Unlock()

	// 重置序号
	s.seqMu.Lock()
	s.sendSeq = 1
	s.recvAckSeq = 0
	s.seqMu.Unlock()

	s.connected = true

	// 延时10ms发送DC3
	time.AfterFunc(10*time.Millisecond, func() {
		s.sendDC3()
	})
}

// handleRSR 处理RSR系统工作状态请求
// 延时10毫秒回复RSR（根据主备状态变量和控制模式变量填充报文）
func (s *CBISimulator) handleRSR(frame *protocol.Frame) {
	time.AfterFunc(10*time.Millisecond, func() {
		s.sendRSR()
	})
}

// handleACK 处理ACK帧
// ACK计数器递增1后判断：
// - ACK计数器=1：回复SDI
// - ACK计数器=2：回复ACQ
// - ACK计数器=10：回复TSQ
// - ACK计数器>10且为3的倍数：回复SDCI
// - ACK计数器>10且为5的倍数：回复FIR
// - 其他：延时10ms回复ACK
func (s *CBISimulator) handleACK(frame *protocol.Frame) {
	s.mu.Lock()
	s.ackCount++
	count := s.ackCount
	s.mu.Unlock()

	log.Debugf("ACK count: %d", count)

	switch {
	case count == 1:
		// 第一次ACK：回复SDI
		s.sendSDI()

	case count == 2:
		// 第二次ACK：回复ACQ
		s.sendACQ()

	case count == 10:
		// 第十次ACK：回复TSQ
		s.sendTSQ()

	case count > 10 && count%3 == 0:
		// 3的倍数（大于10）：回复SDCI
		s.sendSDCI()

	case count > 10 && count%5 == 0:
		// 5的倍数（大于10）：回复FIR
		s.sendFIR()

	default:
		// 其他：延时10ms回复ACK
		time.AfterFunc(10*time.Millisecond, func() {
			s.sendACK()
		})
	}
}

// handleACA 处理ACA自律控制同意
// 如果ACA同意自律控制，将控制模式变量置为自律控制（0x55），延时10ms回复ACK
func (s *CBISimulator) handleACA(frame *protocol.Frame) {
	if len(frame.Data) > 0 && frame.Data[0] == ControlModeAuto {
		s.mu.Lock()
		s.controlMode = ControlModeAuto
		s.mu.Unlock()
		log.Info("Control mode changed to auto")
	}

	time.AfterFunc(10*time.Millisecond, func() {
		s.sendACK()
	})
}

// handleTSD 处理TSD时间同步数据
// 延时10毫秒回复ACK
func (s *CBISimulator) handleTSD(frame *protocol.Frame) {
	time.AfterFunc(10*time.Millisecond, func() {
		s.sendACK()
	})
}

// handleBCC 处理BCC按钮控制命令
// 延时10毫秒回复ACK
func (s *CBISimulator) handleBCC(frame *protocol.Frame) {
	time.AfterFunc(10*time.Millisecond, func() {
		s.sendACK()
	})
}

// handleSDIQ 处理SDIQ站场数据请求
// 延时10毫秒回复SDI
func (s *CBISimulator) handleSDIQ(frame *protocol.Frame) {
	time.AfterFunc(10*time.Millisecond, func() {
		s.sendSDI()
	})
}

// handleNACK 处理NACK否定应答
func (s *CBISimulator) handleNACK(frame *protocol.Frame) {
	log.Warn("Received NACK, resending last frame")
	// 实际实现中需要重发上一帧
}

// checkAndUpdateSequence 检查并更新序号
// 1. 将发送序号变量的值复制到帧的发送序号字段
// 2. 将接收确认序号变量的值复制到帧的确认序号字段
// 3. 将收到的确认序号与发送序号变量对比，相等则发送序号变量递增1
// 4. 将收到的发送序号与接收确认序号变量对比：
//    - 差值=1：接收确认序号变量递增1后回复ACK
//    - 差值=0：直接回复ACK
//    - 其他：判定通信中断，断开连接
func (s *CBISimulator) checkAndUpdateSequence(frame *protocol.Frame) bool {
	s.seqMu.Lock()
	defer s.seqMu.Unlock()

	// 检查确认序号（对方确认了我发送的帧）
	// 收到的确认序号与发送序号变量对比
	// 注意：ACK帧的确认序号表示对方已确认的帧序号
	if frame.AckSeq == s.sendSeq {
		// 确认序号匹配，发送序号递增
		s.sendSeq++
		if s.sendSeq == 0 {
			s.sendSeq = 1 // 序号从1开始，0无效
		}
	}

	// 检查发送序号（对方发送的帧序号）
	// 将收到的发送序号与接收确认序号变量对比
	seqDiff := int(frame.SendSeq) - int(s.recvAckSeq)
	if seqDiff < 0 {
		seqDiff += 256 // 处理序号回绕
	}

	switch {
	case seqDiff == 1:
		// 正常帧，序号递增后回复ACK
		s.recvAckSeq = frame.SendSeq
		// 对于非ACK帧，由具体处理函数决定是否回复
		if frame.Type == protocol.ACK {
			return true // ACK帧不触发回复
		}
		return true

	case seqDiff == 0:
		// 重复帧，直接回复ACK
		log.Warnf("Duplicate frame: %s (seq=%d)", frame.Type, frame.SendSeq)
		go s.sendACK()
		return false // 不继续处理

	default:
		// 序号跳变/丢失，通信中断
		log.Errorf("Sequence error: expected %d, got %d, disconnecting", s.recvAckSeq+1, frame.SendSeq)
		s.disconnect()
		return false
	}
}

// disconnect 断开连接
func (s *CBISimulator) disconnect() {
	s.mu.Lock()
	s.connected = false
	s.ackCount = 0
	s.mu.Unlock()

	if s.transport != nil {
		s.transport.Stop()
	}
}

// === 发送帧方法 ===

// sendFrame 发送帧（核心发送逻辑）
func (s *CBISimulator) sendFrame(frame *protocol.Frame) error {
	s.seqMu.Lock()
	// 将发送序号变量复制到帧
	frame.SendSeq = s.sendSeq
	// 将接收确认序号变量复制到帧
	frame.AckSeq = s.recvAckSeq
	s.seqMu.Unlock()

	// 设置版本号
	frame.Version = protocol.Version

	// 编码发送
	data, err := protocol.EncodeFrame(frame)
	if err != nil {
		return err
	}

	log.Infof("Sending frame: %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)

	return s.transport.Send(data)
}

// sendDC3 发送DC3连接确认
func (s *CBISimulator) sendDC3() {
	frame := &protocol.Frame{
		Type:    protocol.DC3,
		SendSeq: 0x00, // DC3帧序号为0
		AckSeq:  0x00,
	}
	s.sendFrame(frame)
}

// sendACK 发送ACK
func (s *CBISimulator) sendACK() error {
	return s.sendFrame(&protocol.Frame{Type: protocol.ACK})
}

// sendNACK 发送NACK
func (s *CBISimulator) sendNACK() error {
	return s.sendFrame(&protocol.Frame{Type: protocol.NACK})
}

// sendVERROR 发送VERROR
func (s *CBISimulator) sendVERROR() error {
	return s.sendFrame(&protocol.Frame{Type: protocol.VERROR})
}

// sendRSR 发送RSR系统工作状态
func (s *CBISimulator) sendRSR() error {
	s.mu.RLock()
	role := s.roleState
	mode := s.controlMode
	s.mu.RUnlock()

	// RSR数据：主备状态 + 控制模式
	data := []byte{role, mode}
	return s.sendFrame(&protocol.Frame{
		Type: protocol.RSR,
		Data: data,
	})
}

// sendSDI 发送SDI完整站场数据
func (s *CBISimulator) sendSDI() error {
	var data []byte
	if s.stationMgr != nil {
		// 使用站场配置构建数据
		s.stationMgr.RandomizeStates()
		data = s.stationMgr.BuildSDIData()
	} else {
		// 使用随机数据
		data = s.generateRandomSDIData()
	}
	return s.sendFrame(&protocol.Frame{
		Type: protocol.SDI,
		Data: data,
	})
}

// sendSDCI 发送SDCI增量数据
func (s *CBISimulator) sendSDCI() error {
	var data []byte
	if s.stationMgr != nil {
		data = s.stationMgr.BuildSDCIData()
	} else {
		data = s.generateRandomSDCIData()
	}
	return s.sendFrame(&protocol.Frame{
		Type: protocol.SDCI,
		Data: data,
	})
}

// sendFIR 发送FIR故障信息报告
func (s *CBISimulator) sendFIR() error {
	data := s.generateFIRData()
	return s.sendFrame(&protocol.Frame{
		Type: protocol.FIR,
		Data: data,
	})
}

// sendACQ 发送ACQ自律控制请求
func (s *CBISimulator) sendACQ() error {
	data := []byte{ControlModeAuto} // 请求自律控制
	return s.sendFrame(&protocol.Frame{
		Type: protocol.ACQ,
		Data: data,
	})
}

// sendTSQ 发送TSQ时间同步请求
func (s *CBISimulator) sendTSQ() error {
	return s.sendFrame(&protocol.Frame{Type: protocol.TSQ})
}

// === 数据生成方法 ===

// generateRandomSDIData 生成随机SDI数据
func (s *CBISimulator) generateRandomSDIData() []byte {
	// 生成1-10个随机设备数据
	count := rand.Intn(10) + 1
	data := make([]byte, count*3)
	for i := range data {
		data[i] = byte(rand.Intn(256))
	}
	return data
}

// generateRandomSDCIData 生成随机SDCI数据
func (s *CBISimulator) generateRandomSDCIData() []byte {
	// SDCI格式：设备索引(2字节) + 状态字节(1字节)
	data := make([]byte, 3)
	// 随机设备索引
	data[0] = byte(rand.Intn(256))
	data[1] = byte(rand.Intn(256))
	// 随机状态
	data[2] = byte(rand.Intn(256))
	return data
}

// generateFIRData 生成FIR故障数据
func (s *CBISimulator) generateFIRData() []byte {
	// 尝试从配置加载错误码
	errorTable, err := station.LoadErrorCodeTable("configs/Error.sys")
	if err != nil || errorTable == nil {
		log.Warnf("Failed to load error codes: %v", err)
		// 使用默认错误数据
		return []byte{0, 0, 0, 0, 0}
	}

	errors := errorTable.GetAllErrors()
	if len(errors) == 0 {
		return []byte{0, 0, 0, 0, 0}
	}

	// 随机选择错误码
	errCode := errors[rand.Intn(len(errors))]

	// 构造FIR数据：故障类型(1字节) + 设备索引(2字节) + 故障码(2字节)
	data := make([]byte, 5)
	data[0] = byte(rand.Intn(10) + 1)           // 故障类型1-10
	data[1] = byte(rand.Intn(256))              // 设备索引低字节
	data[2] = byte(rand.Intn(256))              // 设备索引高字节
	data[3] = byte(errCode.Code & 0xFF)         // 故障码低字节
	data[4] = byte((errCode.Code >> 8) & 0xFF)  // 故障码高字节

	return data
}

// === 公开方法 ===

// SetOnFrameReceived 设置帧接收回调
func (s *CBISimulator) SetOnFrameReceived(callback func(frame *protocol.Frame)) {
	s.onFrameReceived = callback
}

// GetStationManager 获取站场状态管理器
func (s *CBISimulator) GetStationManager() *station.StationStateManager {
	return s.stationMgr
}

// SendFrame 发送帧（公开方法）
func (s *CBISimulator) SendFrame(frame *protocol.Frame) error {
	return s.sendFrame(frame)
}

// GetState 获取当前状态
func (s *CBISimulator) GetState() (role byte, mode byte, ackCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.roleState, s.controlMode, s.ackCount
}

// IsConnected 返回连接状态
func (s *CBISimulator) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}