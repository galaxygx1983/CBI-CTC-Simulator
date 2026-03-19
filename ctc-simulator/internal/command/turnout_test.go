// internal/command/turnout_test.go
package command

import (
	"testing"

	"ctc-simulator/internal/protocol"
)

func TestBuildTurnoutCommand(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name      string
		index     uint16
		operation TurnoutOperation
	}{
		{"Normal operation", 0x0100, TurnoutOpNormal},
		{"Reverse operation", 0x0101, TurnoutOpReverse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := builder.BuildTurnoutCommand(tt.index, tt.operation)
			if frame == nil {
				t.Fatal("BuildTurnoutCommand returned nil")
			}

			// 验证帧类型
			if frame.Type != protocol.BCC {
				t.Errorf("Expected BCC frame type, got %s", frame.Type)
			}

			// 验证数据长度
			if frame.DataLength != 3 {
				t.Errorf("Expected 3 bytes of data, got %d", frame.DataLength)
			}

			// 验证操作码
			if frame.Data[2] != byte(tt.operation) {
				t.Errorf("Expected operation %d, got %d", tt.operation, frame.Data[2])
			}
		})
	}
}