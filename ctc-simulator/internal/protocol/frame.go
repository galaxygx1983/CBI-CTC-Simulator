// internal/protocol/frame.go
package protocol

import (
	"errors"
	"fmt"
)

// FrameType 帧类型
type FrameType uint8

// 通信控制帧
const (
	DC2    FrameType = 0x12 // 连接请求
	DC3    FrameType = 0x13 // 连接确认
	ACK    FrameType = 0x06 // 应答/心跳
	NACK   FrameType = 0x15 // 否定应答
	VERROR FrameType = 0x10 // 版本错误
)

// 数据传送帧
const (
	SDCI FrameType = 0x8A // 站场数据变化(增量)
	SDI  FrameType = 0x85 // 站场完整数据(全量)
	SDIQ FrameType = 0x6A // 站场数据请求
	FIR  FrameType = 0x65 // 故障信息报告
	RSR  FrameType = 0xAA // 系统工作状态报告
	BCC  FrameType = 0x95 // 按钮控制命令
	ACQ  FrameType = 0x75 // 自律控制请求
	ACA  FrameType = 0x7A // 自律控制同意
	TSQ  FrameType = 0x9A // 时间同步请求
	TSD  FrameType = 0xA5 // 时间同步数据
)

// String 返回帧类型名称
func (ft FrameType) String() string {
	names := map[FrameType]string{
		DC2:    "DC2",
		DC3:    "DC3",
		ACK:    "ACK",
		NACK:   "NACK",
		VERROR: "VERROR",
		SDCI:   "SDCI",
		SDI:    "SDI",
		SDIQ:   "SDIQ",
		FIR:    "FIR",
		RSR:    "RSR",
		BCC:    "BCC",
		ACQ:    "ACQ",
		ACA:    "ACA",
		TSQ:    "TSQ",
		TSD:    "TSD",
	}
	if name, ok := names[ft]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN(0x%02X)", uint8(ft))
}

// IsControlFrame 判断是否为控制帧
func (ft FrameType) IsControlFrame() bool {
	return ft == DC2 || ft == DC3 || ft == ACK || ft == NACK || ft == VERROR
}

// Frame 帧结构
type Frame struct {
	Header     byte      // 帧头 0x7D
	HeaderLen  byte      // 首部长 0x04
	Version    byte      // 版本号
	SendSeq    byte      // 发送序号
	AckSeq     byte      // 确认序号
	Type       FrameType // 帧类型
	DataLength uint16    // 数据长度
	Data       []byte    // 数据载荷
	CRC        uint16    // CRC校验
	Tail       byte      // 帧尾 0x7E
}

// 帧常量
const (
	FrameHeader = 0x7D
	FrameTail   = 0x7E
	EscapeChar  = 0x7F
	HeaderLen   = 0x04
	Version     = 0x11
)

// EncodeFrame 编码帧为字节数组
func EncodeFrame(frame *Frame) ([]byte, error) {
	if frame == nil {
		return nil, errors.New("frame is nil")
	}

	// 构建帧体（不含帧头帧尾）
	body := make([]byte, 0)

	// 首部长
	body = append(body, HeaderLen)

	// 版本号
	version := byte(Version)
	if frame.Version != 0 {
		version = frame.Version
	}
	body = append(body, version)

	// 发送序号
	body = append(body, frame.SendSeq)

	// 确认序号
	body = append(body, frame.AckSeq)

	// 帧类型
	body = append(body, byte(frame.Type))

	// 数据长度和数据（如果有数据）
	if len(frame.Data) > 0 {
		dataLen := uint16(len(frame.Data))
		body = append(body, byte(dataLen&0xFF), byte(dataLen>>8)) // 小端序
		body = append(body, frame.Data...)
	}

	// 计算CRC（对帧体）
	crc := CalculateCRC(body)
	body = append(body, byte(crc&0xFF), byte(crc>>8)) // 小端序

	// 转义帧体
	escapedBody := EscapeData(body)

	// 组装完整帧
	result := make([]byte, 0, len(escapedBody)+2)
	result = append(result, FrameHeader)
	result = append(result, escapedBody...)
	result = append(result, FrameTail)

	return result, nil
}