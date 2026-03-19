// internal/command/builder.go
package command

import (
	"sync"

	"ctc-simulator/internal/protocol"
)

// 道岔操作命令
const (
	TurnoutNormal  byte = 0x01 // 定位
	TurnoutReverse byte = 0x02 // 反位
)

// 信号机命令
const (
	SignalClose byte = 0x00 // 关闭
	SignalOpen  byte = 0x01 // 开放
)

// 帧常量
const (
	FrameHeaderLen = 0x04 // 首部长度固定为4
	FrameVersion   = 0x11 // 协议版本固定为0x11
)

// Builder 命令构建器
// sendSeq: 发送序号变量（收到确认后才递增）
// ackSeq: 接收确认序号变量
type Builder struct {
	sendSeq byte
	ackSeq  byte
	mu      sync.Mutex
}

// NewBuilder 创建命令构建器
func NewBuilder() *Builder {
	return &Builder{
		sendSeq: 1, // 序号从1开始
	}
}

// BuildBCC 构建BCC控制命令
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildBCC(deviceIndex uint16, command byte) *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	// BCC数据: 设备索引(2字节大端) + 控制命令(1字节)
	data := make([]byte, 3)
	data[0] = byte(deviceIndex >> 8)   // 高字节
	data[1] = byte(deviceIndex & 0xFF) // 低字节
	data[2] = command

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:       protocol.BCC,
		SendSeq:    b.sendSeq,
		AckSeq:     b.ackSeq,
		DataLength: uint16(len(data)),
		Data:       data,
	}

	return frame
}

// BuildACA 构建自律控制同意响应
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildACA(agreed bool) *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	// ACA数据: 0x55=同意, 0xAA=拒绝
	var data []byte
	if agreed {
		data = []byte{0x55}
	} else {
		data = []byte{0xAA}
	}

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:       protocol.ACA,
		SendSeq:    b.sendSeq,
		AckSeq:     b.ackSeq,
		DataLength: uint16(len(data)),
		Data:       data,
	}

	return frame
}

// BuildRSR 构建状态报告
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildRSR(isMaster bool) *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	// RSR数据: 主备状态(1字节) + 控制模式(1字节)
	data := make([]byte, 2)
	if isMaster {
		data[0] = 0x55 // 主机
	} else {
		data[0] = 0xAA // 备机
	}
	data[1] = 0x55 // 自律控制模式

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:       protocol.RSR,
		SendSeq:    b.sendSeq,
		AckSeq:     b.ackSeq,
		DataLength: uint16(len(data)),
		Data:       data,
	}

	return frame
}

// BuildACK 构建确认帧
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildACK() *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:    protocol.ACK,
		SendSeq: b.sendSeq,
		AckSeq:  b.ackSeq,
	}

	return frame
}

// BuildSDI 构建站场初始数据帧
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildSDI(data []byte) *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:       protocol.SDI,
		SendSeq:    b.sendSeq,
		AckSeq:     b.ackSeq,
		DataLength: uint16(len(data)),
		Data:       data,
	}

	return frame
}

// BuildDC3 构建连接确认帧
// 注意：DC3是连接帧，不受序号控制，发送序号和确认序号均为0x00
func (b *Builder) BuildDC3() *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	// DC3序号强制为0x00，不使用sendSeq/ackSeq
	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:    protocol.DC3,
		SendSeq: 0x00,
		AckSeq:  0x00,
	}

	// DC3发送后，初始化序号为1（为后续数据帧准备）
	b.sendSeq = 1
	b.ackSeq = 0

	return frame
}

// BuildSDCI 构建增量站场数据帧
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildSDCI(data []byte) *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:       protocol.SDCI,
		SendSeq:    b.sendSeq,
		AckSeq:     b.ackSeq,
		DataLength: uint16(len(data)),
		Data:       data,
	}

	return frame
}

// BuildTSQ 构建时间同步请求帧
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildTSQ() *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:    protocol.TSQ,
		SendSeq: b.sendSeq,
		AckSeq:  b.ackSeq,
	}

	return frame
}

// BuildSDIQ 构建站场数据请求帧
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildSDIQ() *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:    protocol.SDIQ,
		SendSeq: b.sendSeq,
		AckSeq:  b.ackSeq,
	}

	return frame
}

// BuildTSD 构建时间同步数据帧
// 发送序号 = sendSeq，确认序号 = ackSeq（不递增sendSeq，等收到确认后递增）
func (b *Builder) BuildTSD(timestamp uint64) *protocol.Frame {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 时间戳8字节
	data := make([]byte, 8)
	data[0] = byte(timestamp)
	data[1] = byte(timestamp >> 8)
	data[2] = byte(timestamp >> 16)
	data[3] = byte(timestamp >> 24)
	data[4] = byte(timestamp >> 32)
	data[5] = byte(timestamp >> 40)
	data[6] = byte(timestamp >> 48)
	data[7] = byte(timestamp >> 56)

	frame := &protocol.Frame{
		HeaderLen: FrameHeaderLen,
		Version:   FrameVersion,
		Type:       protocol.TSD,
		SendSeq:    b.sendSeq,
		AckSeq:     b.ackSeq,
		DataLength: 8,
		Data:       data,
	}

	return frame
}

// UpdateAckSeq 更新接收确认序号变量
func (b *Builder) UpdateAckSeq(seq byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ackSeq = seq
}

// UpdateSendSeq 更新发送序号变量
func (b *Builder) UpdateSendSeq(seq byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendSeq = seq
}

// ConfirmAck 确认序号验证：检查确认序号是否匹配发送序号，匹配则递增
func (b *Builder) ConfirmAck(ackSeq byte) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ackSeq == b.sendSeq {
		// 帧已被确认，发送序号递增
		b.sendSeq++
		if b.sendSeq == 0 {
			b.sendSeq = 1 // 序号0保留
		}
		return true
	}
	return false
}

// GetSendSeq 获取当前发送序号
func (b *Builder) GetSendSeq() byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sendSeq
}

// GetAckSeq 获取当前接收确认序号
func (b *Builder) GetAckSeq() byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ackSeq
}
