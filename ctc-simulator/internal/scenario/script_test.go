// internal/scenario/script_test.go
package scenario

import (
	"encoding/json"
	"testing"
)

func TestScriptJSON(t *testing.T) {
	jsonStr := `{
		"name": "test",
		"description": "test scenario",
		"actions": [
			{
				"delay_ms": 1000,
				"type": "BCC",
				"device_index": 256,
				"command": "TURNOUT_NORMAL"
			}
		]
	}`

	var script Script
	err := json.Unmarshal([]byte(jsonStr), &script)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if script.Name != "test" {
		t.Errorf("Expected name 'test', got %s", script.Name)
	}

	if len(script.Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(script.Actions))
	}

	if script.Actions[0].DelayMs != 1000 {
		t.Errorf("Expected delay 1000, got %d", script.Actions[0].DelayMs)
	}
}