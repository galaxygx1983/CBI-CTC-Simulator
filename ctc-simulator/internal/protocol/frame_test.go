// internal/protocol/frame_test.go
package protocol

import "testing"

func TestFrameTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		ft       FrameType
		expected uint8
	}{
		{"DC2", DC2, 0x12},
		{"DC3", DC3, 0x13},
		{"ACK", ACK, 0x06},
		{"NACK", NACK, 0x15},
		{"SDCI", SDCI, 0x8A},
		{"SDI", SDI, 0x85},
		{"RSR", RSR, 0xAA},
		{"BCC", BCC, 0x95},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if uint8(tt.ft) != tt.expected {
				t.Errorf("%s: expected 0x%02X, got 0x%02X", tt.name, tt.expected, tt.ft)
			}
		})
	}
}

func TestFrameTypeName(t *testing.T) {
	if DC2.String() != "DC2" {
		t.Errorf("Expected 'DC2', got '%s'", DC2.String())
	}
}

func TestIsControlFrame(t *testing.T) {
	if !DC2.IsControlFrame() {
		t.Error("DC2 should be a control frame")
	}
	if !ACK.IsControlFrame() {
		t.Error("ACK should be a control frame")
	}
	if SDCI.IsControlFrame() {
		t.Error("SDCI should not be a control frame")
	}
}