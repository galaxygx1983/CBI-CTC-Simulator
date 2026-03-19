// internal/protocol/decoder_test.go
package protocol

import (
	"testing"
)

func TestDecodeDC2Frame(t *testing.T) {
	// 先编码一个DC2帧，再解码，验证编解码一致性
	original := &Frame{Type: DC2}

	encoded, _ := EncodeFrame(original)
	frame, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("Decode DC2 failed: %v", err)
	}

	if frame.Type != DC2 {
		t.Errorf("Frame type: expected DC2, got %s", frame.Type)
	}
	if frame.HeaderLen != 0x04 {
		t.Errorf("Header length: expected 0x04, got 0x%02X", frame.HeaderLen)
	}
	if frame.Version != 0x11 {
		t.Errorf("Version: expected 0x11, got 0x%02X", frame.Version)
	}
}

func TestDecodeACKFrame(t *testing.T) {
	// 构造ACK帧测试编解码一致性
	original := &Frame{
		SendSeq: 5,
		AckSeq:  3,
		Type:    ACK,
	}

	encoded, _ := EncodeFrame(original)
	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("Decode ACK failed: %v", err)
	}

	if decoded.SendSeq != original.SendSeq {
		t.Errorf("SendSeq: expected %d, got %d", original.SendSeq, decoded.SendSeq)
	}
	if decoded.AckSeq != original.AckSeq {
		t.Errorf("AckSeq: expected %d, got %d", original.AckSeq, decoded.AckSeq)
	}
}

func TestDecodeSDCIFrame(t *testing.T) {
	payload := []byte{0x00, 0x25, 0x01, 0x00, 0x26, 0x02} // 两个设备状态
	original := &Frame{
		Type:       SDCI,
		SendSeq:    1,
		AckSeq:     0,
		DataLength: uint16(len(payload)),
		Data:       payload,
	}

	encoded, _ := EncodeFrame(original)
	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("Decode SDCI failed: %v", err)
	}

	if decoded.Type != SDCI {
		t.Errorf("Type: expected SDCI, got %s", decoded.Type)
	}
	if decoded.DataLength != uint16(len(payload)) {
		t.Errorf("DataLength: expected %d, got %d", len(payload), decoded.DataLength)
	}
	if len(decoded.Data) != len(payload) {
		t.Errorf("Data length: expected %d, got %d", len(payload), len(decoded.Data))
	}
}

func TestDecodeInvalidFrame(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"no header", []byte{0x04, 0x11, 0x00, 0x00, 0x12, 0x49, 0xF7, 0x7E}},
		{"no tail", []byte{0x7D, 0x04, 0x11, 0x00, 0x00, 0x12, 0x49, 0xF7}},
		{"too short", []byte{0x7D, 0x7E}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeFrame(tt.data)
			if err == nil {
				t.Error("Expected error for invalid frame")
			}
		})
	}
}

func TestDecodeFrameWithEscape(t *testing.T) {
	// 包含转义字节的帧
	original := &Frame{
		Type:       SDCI,
		DataLength: 3,
		Data:       []byte{0x7D, 0x7E, 0x7F},
	}

	encoded, _ := EncodeFrame(original)
	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("Decode escaped frame failed: %v", err)
	}

	if decoded.Data[0] != 0x7D {
		t.Errorf("Data[0]: expected 0x7D, got 0x%02X", decoded.Data[0])
	}
	if decoded.Data[1] != 0x7E {
		t.Errorf("Data[1]: expected 0x7E, got 0x%02X", decoded.Data[1])
	}
	if decoded.Data[2] != 0x7F {
		t.Errorf("Data[2]: expected 0x7F, got 0x%02X", decoded.Data[2])
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// 测试多种帧类型的编解码往返
	frames := []*Frame{
		{Type: DC2},
		{Type: DC3},
		{Type: ACK, SendSeq: 5, AckSeq: 3},
		{Type: NACK, SendSeq: 1, AckSeq: 0},
		{Type: SDCI, SendSeq: 1, DataLength: 3, Data: []byte{0x01, 0x02, 0x03}},
		{Type: SDI, SendSeq: 2, DataLength: 2, Data: []byte{0x00, 0x00}},
	}

	for i, original := range frames {
		t.Run(original.Type.String(), func(t *testing.T) {
			encoded, err := EncodeFrame(original)
			if err != nil {
				t.Fatalf("Encode frame %d failed: %v", i, err)
			}

			decoded, err := DecodeFrame(encoded)
			if err != nil {
				t.Fatalf("Decode frame %d failed: %v", i, err)
			}

			if decoded.Type != original.Type {
				t.Errorf("Frame %d: type mismatch", i)
			}
			if decoded.SendSeq != original.SendSeq {
				t.Errorf("Frame %d: SendSeq mismatch: expected %d, got %d", i, original.SendSeq, decoded.SendSeq)
			}
			if decoded.AckSeq != original.AckSeq {
				t.Errorf("Frame %d: AckSeq mismatch: expected %d, got %d", i, original.AckSeq, decoded.AckSeq)
			}
		})
	}
}