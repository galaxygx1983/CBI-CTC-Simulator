// internal/scenario/loader.go
package scenario

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadScript 从文件加载场景脚本
func LoadScript(path string) (*Script, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}

	var script Script
	if err := json.Unmarshal(data, &script); err != nil {
		return nil, fmt.Errorf("parse json failed: %w", err)
	}

	return &script, nil
}

// SaveScript 保存场景脚本到文件
func SaveScript(path string, script *Script) error {
	data, err := json.MarshalIndent(script, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json failed: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}

	return nil
}