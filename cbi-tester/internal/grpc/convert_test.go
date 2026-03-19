// internal/grpc/convert_test.go
package grpc

import (
	"testing"

	pb "cbi-simulator/internal/pb/cbi/v1"
	"cbi-simulator/internal/protocol"
)

func TestFrameToProtoFrame(t *testing.T) {
	frame := &protocol.Frame{
		Type:    protocol.DC2,
		SendSeq: 1,
		AckSeq:  0,
	}

	protoFrame := frameToProtoFrame(frame)

	if protoFrame.Type != pb.FrameType_FRAME_TYPE_DC2 {
		t.Errorf("Expected DC2, got %v", protoFrame.Type)
	}
	if protoFrame.SendSeq != 1 {
		t.Errorf("Expected SendSeq 1, got %d", protoFrame.SendSeq)
	}
}

func TestProtoFrameToFrame(t *testing.T) {
	protoFrame := &pb.Frame{
		Type:    pb.FrameType_FRAME_TYPE_DC3,
		SendSeq: 2,
		AckSeq:  1,
	}

	frame := protoFrameToFrame(protoFrame)

	if frame.Type != protocol.DC3 {
		t.Errorf("Expected DC3, got %s", frame.Type)
	}
	if frame.SendSeq != 2 {
		t.Errorf("Expected SendSeq 2, got %d", frame.SendSeq)
	}
}

func TestDataFrameConversion(t *testing.T) {
	// 测试数据帧转换
	frame := &protocol.Frame{
		Type:       protocol.SDCI,
		SendSeq:    1,
		AckSeq:     0,
		DataLength: 3,
		Data:       []byte{0x00, 0x25, 0x01},
	}

	protoFrame := frameToProtoFrame(frame)

	if protoFrame.DataLength == nil || *protoFrame.DataLength != 3 {
		t.Errorf("Expected DataLength 3")
	}
	if len(protoFrame.Data) != 3 {
		t.Errorf("Expected Data length 3")
	}

	// 往返转换
	backFrame := protoFrameToFrame(protoFrame)
	if backFrame.Type != frame.Type {
		t.Errorf("Type mismatch")
	}
	if backFrame.DataLength != frame.DataLength {
		t.Errorf("DataLength mismatch")
	}
}

func TestControlFrameConversion(t *testing.T) {
	// 测试控制帧（无数据）
	frame := &protocol.Frame{
		Type:    protocol.ACK,
		SendSeq: 5,
		AckSeq:  3,
	}

	protoFrame := frameToProtoFrame(frame)

	if protoFrame.DataLength != nil {
		t.Errorf("Control frame should not have DataLength")
	}
	if len(protoFrame.Data) != 0 {
		t.Errorf("Control frame should not have Data")
	}
}

func TestAllFrameTypeConversions(t *testing.T) {
	// 测试所有帧类型的转换
	frameTypes := []protocol.FrameType{
		protocol.DC2, protocol.DC3, protocol.ACK, protocol.NACK,
		protocol.VERROR, protocol.SDCI, protocol.SDI, protocol.SDIQ,
		protocol.FIR, protocol.RSR, protocol.BCC, protocol.ACQ,
		protocol.ACA, protocol.TSQ, protocol.TSD,
	}

	for _, ft := range frameTypes {
		frame := &protocol.Frame{
			Type:    ft,
			SendSeq: 1,
			AckSeq:  0,
		}

		protoFrame := frameToProtoFrame(frame)
		backFrame := protoFrameToFrame(protoFrame)

		if backFrame.Type != frame.Type {
			t.Errorf("Frame type %s not preserved during conversion", ft)
		}
	}
}

func TestUnknownFrameTypeConversion(t *testing.T) {
	// 测试未知帧类型
	protoFrame := &pb.Frame{
		Type:    pb.FrameType_FRAME_TYPE_UNSPECIFIED,
		SendSeq: 1,
		AckSeq:  0,
	}

	frame := protoFrameToFrame(protoFrame)

	// 未知类型应该返回 FrameType(0xFF)
	if frame.Type != protocol.FrameType(0xFF) {
		t.Errorf("Expected unknown frame type (0xFF), got %v", frame.Type)
	}
}

func TestFrameWithHeaderAndVersion(t *testing.T) {
	frame := &protocol.Frame{
		Type:      protocol.DC2,
		Header:    0x7E,
		HeaderLen: 4,
		Version:   1,
		SendSeq:   10,
		AckSeq:    5,
		CRC:       0x1234,
	}

	protoFrame := frameToProtoFrame(frame)

	if protoFrame.Header[0] != 0x7E {
		t.Errorf("Header not preserved")
	}
	if protoFrame.HeaderLen != 4 {
		t.Errorf("HeaderLen not preserved")
	}
	if protoFrame.Version != 1 {
		t.Errorf("Version not preserved")
	}
	if protoFrame.SendSeq != 10 {
		t.Errorf("SendSeq not preserved")
	}
	if protoFrame.AckSeq != 5 {
		t.Errorf("AckSeq not preserved")
	}
	if protoFrame.Crc != 0x1234 {
		t.Errorf("CRC not preserved")
	}

	// 往返转换
	backFrame := protoFrameToFrame(protoFrame)
	if backFrame.SendSeq != frame.SendSeq {
		t.Errorf("SendSeq mismatch after round trip")
	}
}

func TestEmptyDataFrame(t *testing.T) {
	// 测试空数据的数据帧
	frame := &protocol.Frame{
		Type:       protocol.SDCI,
		DataLength: 0,
		Data:       []byte{},
	}

	protoFrame := frameToProtoFrame(frame)

	// 空数据不应该设置DataLength
	if protoFrame.DataLength != nil {
		t.Errorf("Empty data frame should not have DataLength")
	}
}

func TestLargeDataFrame(t *testing.T) {
	// 测试大数据帧
	largeData := make([]byte, 1000)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	frame := &protocol.Frame{
		Type:       protocol.SDCI,
		DataLength: uint16(len(largeData)),
		Data:       largeData,
	}

	protoFrame := frameToProtoFrame(frame)
	backFrame := protoFrameToFrame(protoFrame)

	if len(backFrame.Data) != len(frame.Data) {
		t.Errorf("Data length mismatch: expected %d, got %d", len(frame.Data), len(backFrame.Data))
	}

	// 验证数据内容
	for i := range frame.Data {
		if backFrame.Data[i] != frame.Data[i] {
			t.Errorf("Data mismatch at index %d", i)
			break
		}
	}
}