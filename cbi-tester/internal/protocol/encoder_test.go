// internal/protocol/encoder_test.go
package protocol

import (
	"testing"
)

func TestEncodeDC2Frame(t *testing.T) {
	frame := &Frame{
		Type: DC2,
	}

	data, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("Encode DC2 failed: %v", err)
	}

	// DC2帧应该是9字节: 7D 04 11 00 00 12 CRC_L CRC_H 7E
	if len(data) != 9 {
		t.Errorf("DC2 frame length: expected 9, got %d", len(data))
	}

	if data[0] != FrameHeader {
		t.Errorf("Frame header: expected 0x7D, got 0x%02X", data[0])
	}

	if data[8] != FrameTail {
		t.Errorf("Frame tail: expected 0x7E, got 0x%02X", data[8])
	}

	if data[5] != byte(DC2) {
		t.Errorf("Frame type: expected 0x12, got 0x%02X", data[5])
	}
}

func TestEncodeDC3Frame(t *testing.T) {
	frame := &Frame{
		Type: DC3,
	}

	data, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("Encode DC3 failed: %v", err)
	}

	if data[5] != byte(DC3) {
		t.Errorf("Frame type: expected 0x13, got 0x%02X", data[5])
	}
}

func TestEncodeACKFrame(t *testing.T) {
	frame := &Frame{
		SendSeq: 5,
		AckSeq:  3,
		Type:    ACK,
	}

	data, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("Encode ACK failed: %v", err)
	}

	if data[3] != 5 {
		t.Errorf("SendSeq: expected 5, got %d", data[3])
	}
	if data[4] != 3 {
		t.Errorf("AckSeq: expected 3, got %d", data[4])
	}
}

func TestEncodeSDCIFrame(t *testing.T) {
	// SDCI帧包含数据载荷
	payload := []byte{0x00, 0x25, 0x01} // 设备索引37，状态0x01
	frame := &Frame{
		Type:       SDCI,
		SendSeq:    1,
		AckSeq:     0,
		DataLength: uint16(len(payload)),
		Data:       payload,
	}

	data, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("Encode SDCI failed: %v", err)
	}

	// 9(固定) + 2(长度) + 3(数据) = 14字节
	if len(data) != 14 {
		t.Errorf("SDCI frame length: expected 14, got %d", len(data))
	}
}

func TestEncodeFrameWithEscape(t *testing.T) {
	// 测试包含需要转义字节的帧
	payload := []byte{0x7D, 0x7E, 0x7F} // 这些字节需要转义
	frame := &Frame{
		Type:       SDCI,
		DataLength: uint16(len(payload)),
		Data:       payload,
	}

	data, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("Encode frame with escape failed: %v", err)
	}

	// 验证帧头帧尾未被转义
	if data[0] != FrameHeader {
		t.Error("Frame header should not be escaped")
	}
	if data[len(data)-1] != FrameTail {
		t.Error("Frame tail should not be escaped")
	}
}

func TestEncodeNilFrame(t *testing.T) {
	_, err := EncodeFrame(nil)
	if err == nil {
		t.Error("EncodeFrame should return error for nil frame")
	}
}