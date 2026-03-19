// internal/protocol/decoder.go
package protocol

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidFrameLength = errors.New("invalid frame length")
	ErrInvalidHeader       = errors.New("invalid frame header")
	ErrInvalidTail         = errors.New("invalid frame tail")
	ErrCRCMismatch         = errors.New("CRC mismatch")
)

// DecodeFrame 从字节数组解码帧
func DecodeFrame(data []byte) (*Frame, error) {
	// 基本长度检查
	if len(data) < 9 {
		return nil, ErrInvalidFrameLength
	}

	// 帧头检查
	if data[0] != FrameHeader {
		return nil, ErrInvalidHeader
	}

	// 帧尾检查
	if data[len(data)-1] != FrameTail {
		return nil, ErrInvalidTail
	}

	// 提取帧体（不含帧头帧尾）并反转义
	body := data[1 : len(data)-1]
	unescapedBody := UnescapeData(body)

	// 最小帧体长度检查
	if len(unescapedBody) < 7 { // 首部长(1) + 版本(1) + 序号(2) + 类型(1) + CRC(2)
		return nil, ErrInvalidFrameLength
	}

	// 解析帧体
	frame := &Frame{
		Header: FrameHeader,
		Tail:   FrameTail,
	}

	offset := 0

	// 首部长
	frame.HeaderLen = unescapedBody[offset]
	offset++

	// 版本号
	frame.Version = unescapedBody[offset]
	offset++

	// 发送序号
	frame.SendSeq = unescapedBody[offset]
	offset++

	// 确认序号
	frame.AckSeq = unescapedBody[offset]
	offset++

	// 帧类型
	frame.Type = FrameType(unescapedBody[offset])
	offset++

	// 如果是数据帧，解析数据长度和数据
	if !frame.Type.IsControlFrame() {
		if len(unescapedBody) < offset+2 {
			return nil, ErrInvalidFrameLength
		}
		// 数据长度（小端序）
		frame.DataLength = uint16(unescapedBody[offset]) | uint16(unescapedBody[offset+1])<<8
		offset += 2

		// 数据
		if frame.DataLength > 0 {
			if len(unescapedBody) < offset+int(frame.DataLength)+2 {
				return nil, ErrInvalidFrameLength
			}
			frame.Data = make([]byte, frame.DataLength)
			copy(frame.Data, unescapedBody[offset:offset+int(frame.DataLength)])
			offset += int(frame.DataLength)
		}
	}

	// CRC（小端序）
	if len(unescapedBody) < offset+2 {
		return nil, ErrInvalidFrameLength
	}
	frame.CRC = uint16(unescapedBody[offset]) | uint16(unescapedBody[offset+1])<<8

	// CRC校验（对除CRC外的帧体计算）
	crcData := unescapedBody[:offset]
	calculatedCRC := CalculateCRC(crcData)

	if calculatedCRC != frame.CRC {
		return nil, fmt.Errorf("%w: expected 0x%04X, got 0x%04X", ErrCRCMismatch, calculatedCRC, frame.CRC)
	}

	return frame, nil
}