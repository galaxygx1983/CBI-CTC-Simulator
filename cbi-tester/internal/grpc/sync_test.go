// internal/grpc/sync_test.go
package grpc

import (
	"testing"

	"cbi-simulator/internal/station"
)

func TestSyncDeviceState(t *testing.T) {
	client, _ := NewClient("localhost:50051")

	// 添加设备到本地状态
	dev := &station.Device{
		Index: 1,
		Name:  "1",
		Type:  station.DeviceTurnout,
	}
	client.GetStation().AddDevice(dev)

	// 同步设备状态
	err := client.SyncDeviceState(1, []byte{0x02})
	if err != nil {
		t.Errorf("SyncDeviceState failed: %v", err)
	}

	// 验证状态已更新
	state, err := client.GetStation().GetDeviceState(1)
	if err != nil {
		t.Errorf("GetDeviceState failed: %v", err)
	}
	if state != 0x02 {
		t.Errorf("Expected state 0x02, got 0x%02X", state)
	}
}