// internal/config/config_test.go
package config

import (
	"testing"
)

func TestLoadDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Mode != "tcp" {
		t.Errorf("Expected default mode 'tcp', got '%s'", cfg.Server.Mode)
	}
	if cfg.Protocol.Version != 0x11 {
		t.Errorf("Expected protocol version 0x11, got 0x%02X", cfg.Protocol.Version)
	}
	if cfg.Protocol.AckTimeoutMs != 500 {
		t.Errorf("Expected ACK timeout 500ms, got %d", cfg.Protocol.AckTimeoutMs)
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Default config should be valid: %v", err)
	}

	cfg.Server.Mode = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("Invalid mode should fail validation")
	}
}