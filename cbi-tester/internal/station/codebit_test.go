// internal/station/codebit_test.go
package station

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCodebitFile(t *testing.T) {
	// 创建测试文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.zl")

	content := `//-----------------------------------------------------------------------------------
//[objects：对象索引表],对象名称（F开头的可以忽略），对象索引号
//[zlobjects：SDI Buffer对象位置信息表],对象名称，字节索引（从1开始），位索引
//-----------------------------------------------------------------------------------

[objects]
#,10,0
#,D114,63
#,64,37

[zlobjects]
#,10,1,0
#,D114,8,0
#,64,5,0
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Write test file failed: %v", err)
	}

	// 解析
	table, err := LoadCodebitTable(testFile)
	if err != nil {
		t.Fatalf("LoadCodebitTable failed: %v", err)
	}

	// 验证objects
	if len(table.Objects) != 3 {
		t.Errorf("Expected 3 objects, got %d", len(table.Objects))
	}

	// 验证设备查找
	dev, ok := table.GetDeviceByIndex(63)
	if !ok {
		t.Fatal("Device with index 63 not found")
	}
	if dev.Name != "D114" {
		t.Errorf("Expected name D114, got %s", dev.Name)
	}

	// 验证zlobjects
	dev, ok = table.GetDeviceByName("D114")
	if !ok {
		t.Fatal("Device D114 not found")
	}
	if dev.ByteOffset != 8 {
		t.Errorf("Expected ByteOffset 8, got %d", dev.ByteOffset)
	}
}

func TestDeviceTypeDetection(t *testing.T) {
	tests := []struct {
		name     string
		expected DeviceType
	}{
		{"D114", DeviceSignal},   // D开头数字 = 信号机
		{"D6", DeviceSignal},      // D开头数字 = 信号机
		{"64", DeviceTurnout},     // 纯数字 = 道岔
		{"10", DeviceTurnout},     // 纯数字 = 道岔
		{"DK5", DeviceSection},    // 其他 = 区段
		{"102/104G", DeviceSection}, // 其他 = 区段
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt := DetectDeviceType(tt.name)
			if dt != tt.expected {
				t.Errorf("DetectDeviceType(%s): expected %d, got %d", tt.name, tt.expected, dt)
			}
		})
	}
}