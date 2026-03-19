// internal/scenario/loader_test.go
package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScript(t *testing.T) {
	// 创建临时测试文件
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "test_scenario.json")

	content := `{
		"name": "test",
		"description": "test scenario",
		"actions": [
			{"delay_ms": 1000, "type": "BCC", "device_index": 256, "command": "TURNOUT_NORMAL"}
		]
	}`

	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	defer os.Remove(testFile)

	script, err := LoadScript(testFile)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	if script.Name != "test" {
		t.Errorf("Expected name 'test', got %s", script.Name)
	}

	if len(script.Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(script.Actions))
	}
}