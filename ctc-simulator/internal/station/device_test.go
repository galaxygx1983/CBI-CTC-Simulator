// internal/station/device_test.go
package station

import "testing"

func TestDeviceType(t *testing.T) {
	tests := []struct {
		name     string
		dt       DeviceType
		expected string
	}{
		{"Turnout", DeviceTurnout, "turnout"},
		{"Signal", DeviceSignal, "signal"},
		{"Section", DeviceSection, "section"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dt.String() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, tt.dt.String())
			}
		})
	}
}

func TestTurnoutState(t *testing.T) {
	state := NewTurnoutState()

	if state.IsNormal() {
		t.Error("Initial state should not be normal")
	}

	state.SetNormal()
	if !state.IsNormal() {
		t.Error("State should be normal")
	}

	state.SetReverse()
	if !state.IsReverse() {
		t.Error("State should be reverse")
	}

	state.SetNoIndication()
	if !state.IsNoIndication() {
		t.Error("State should be no indication")
	}
}

func TestSignalState(t *testing.T) {
	state := NewSignalState()

	state.SetGreen()
	if !state.IsGreen() {
		t.Error("State should be green")
	}

	state.SetRed()
	if !state.IsRed() {
		t.Error("State should be red")
	}

	state.SetBlue()
	if !state.IsBlue() {
		t.Error("State should be blue")
	}
}

func TestSectionState(t *testing.T) {
	state := NewSectionState()

	if state.IsOccupied() {
		t.Error("Initial state should not be occupied")
	}

	state.SetOccupied()
	if !state.IsOccupied() {
		t.Error("State should be occupied")
	}

	state.SetClear()
	if state.IsOccupied() {
		t.Error("State should be clear")
	}

	state.SetLocked()
	if !state.IsLocked() {
		t.Error("State should be locked")
	}
}

func TestStationState(t *testing.T) {
	ss := NewStationState()

	// 添加道岔设备
	dev := &Device{
		Index: 1,
		Name:  "1",
		Type:  DeviceTurnout,
	}
	ss.AddDevice(dev)

	// 验证设备已添加
	if len(ss.Devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(ss.Devices))
	}

	if len(ss.Turnouts) != 1 {
		t.Errorf("Expected 1 turnout, got %d", len(ss.Turnouts))
	}

	// 获取设备状态
	state, err := ss.GetDeviceState(1)
	if err != nil {
		t.Errorf("GetDeviceState failed: %v", err)
	}
	if state == 0 {
		t.Error("State should not be zero")
	}
}