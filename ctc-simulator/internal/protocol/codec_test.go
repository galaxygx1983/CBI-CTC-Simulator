// internal/protocol/codec_test.go
package protocol

import (
	"testing"
)

func TestCRC16_XMODEM(t *testing.T) {
	// 测试空数据 - XMODEM CRC 空数据为 0x0000
	crc := CalculateCRC([]byte{})
	if crc != 0x0000 {
		t.Errorf("Empty data CRC: expected 0x0000, got 0x%04X", crc)
	}

	// 测试已知数据 "123456789" - XMODEM CRC 应为 0x31C3
	data := []byte("123456789")
	crc = CalculateCRC(data)
	if crc != 0x31C3 {
		t.Errorf("XMODEM CRC for '123456789': expected 0x31C3, got 0x%04X", crc)
	}

	// 测试 DC2 帧数据部分
	data = []byte{0x04, 0x11, 0x00, 0x00, 0x12}
	crc = CalculateCRC(data)
	// CRC值不应为0
	if crc == 0 {
		t.Error("CRC should not be zero for non-empty data")
	}
}

func TestEscapeData(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "no escape needed",
			input:    []byte{0x01, 0x02, 0x03},
			expected: []byte{0x01, 0x02, 0x03},
		},
		{
			name:     "escape 0x7D",
			input:    []byte{0x01, 0x7D, 0x02},
			expected: []byte{0x01, 0x7F, 0xFD, 0x02},
		},
		{
			name:     "escape 0x7E",
			input:    []byte{0x7E},
			expected: []byte{0x7F, 0xFE},
		},
		{
			name:     "escape 0x7F",
			input:    []byte{0x7F},
			expected: []byte{0x7F, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeData(tt.input)
			if !bytesEqual(result, tt.expected) {
				t.Errorf("Escape: expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestUnescapeData(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "no escape",
			input:    []byte{0x01, 0x02, 0x03},
			expected: []byte{0x01, 0x02, 0x03},
		},
		{
			name:     "unescape 0x7F 0xFD",
			input:    []byte{0x01, 0x7F, 0xFD, 0x02},
			expected: []byte{0x01, 0x7D, 0x02},
		},
		{
			name:     "unescape 0x7F 0xFE",
			input:    []byte{0x7F, 0xFE},
			expected: []byte{0x7E},
		},
		{
			name:     "unescape 0x7F 0xFF",
			input:    []byte{0x7F, 0xFF},
			expected: []byte{0x7F},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnescapeData(tt.input)
			if !bytesEqual(result, tt.expected) {
				t.Errorf("Unescape: expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEscapeUnescapeRoundTrip(t *testing.T) {
	// 包含所有特殊字节的数据
	original := []byte{0x00, 0x7D, 0x7E, 0x7F, 0xFF, 0x7D, 0x00}
	escaped := EscapeData(original)
	unescaped := UnescapeData(escaped)
	if !bytesEqual(original, unescaped) {
		t.Errorf("Round trip failed: original %v, got %v", original, unescaped)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}