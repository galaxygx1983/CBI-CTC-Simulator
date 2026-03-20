// cmd/cbi-client/fault/config_test.go
// 故障注入配置测试
package fault

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestFaultConfig_Defaults(t *testing.T) {
	cfg := NewFaultConfig()

	if cfg.AckTimeout != 0 {
		t.Errorf("expected AckTimeout=0, got %d", cfg.AckTimeout)
	}
	if cfg.ReplyDelay != 0 {
		t.Errorf("expected ReplyDelay=0, got %d", cfg.ReplyDelay)
	}
	if cfg.RandomDrop != 0 {
		t.Errorf("expected RandomDrop=0, got %d", cfg.RandomDrop)
	}
	if cfg.ReplyDrop {
		t.Error("expected ReplyDrop=false")
	}
	if cfg.SeqSkip != 0 {
		t.Errorf("expected SeqSkip=0, got %d", cfg.SeqSkip)
	}
}

func TestFaultConfig_RegisterFlags(t *testing.T) {
	cfg := NewFaultConfig()
	cmd := &cobra.Command{}
	cfg.RegisterFlags(cmd)

	// 验证 flag 已注册
	flags := []string{
		"fault-ack-timeout", "fault-delay", "fault-reply-drop",
		"fault-random-drop", "fault-seq-skip", "fault-seq-stuck",
		"fault-seq-mismatch", "fault-nack-after", "fault-nack-random",
		"fault-verror", "fault-block-dc2", "fault-wrong-version",
		"fault-corrupt-data", "fault-empty-data", "fault-extra-data",
		"fault-disconnect-after", "fault-reconnect-loop", "fault-scenario",
	}

	for _, name := range flags {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("flag %s not registered", name)
		}
	}
}

func TestHasAnyFaultFlag_None(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("fault-ack-timeout", 0, "")
	cmd.Flags().Int("fault-delay", 0, "")
	cmd.Flags().String("fault-scenario", "", "")

	// 不传递任何参数时，应返回 false
	if hasAnyFaultFlag(cmd) {
		t.Error("expected false when no fault flags changed")
	}
}

func TestHasAnyFaultFlag_OneChanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("fault-ack-timeout", 0, "")
	cmd.Flags().String("fault-scenario", "", "")

	// 模拟传递 --fault-ack-timeout 100
	cmd.Flags().Set("fault-ack-timeout", "100")

	if !hasAnyFaultFlag(cmd) {
		t.Error("expected true when at least one fault flag is set")
	}
}

func TestHasAnyFaultFlag_Scenario(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("fault-ack-timeout", 0, "")
	cmd.Flags().String("fault-scenario", "", "")

	// 模拟传递 --fault-scenario network-congestion
	cmd.Flags().Set("fault-scenario", "network-congestion")

	if !hasAnyFaultFlag(cmd) {
		t.Error("expected true when scenario is set")
	}
}

func TestHasAnyFaultFlag_DefaultValue(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("fault-ack-timeout", 0, "")

	// 传递 --fault-ack-timeout 0（默认值），Changed 仍为 false
	// 注意：cobra 的 Changed 只有在显式传递参数时才为 true
	// 使用 Set 会设置 Changed 为 true

	// 重新创建 cmd 测试未设置 Changed 的情况
	cmd2 := &cobra.Command{}
	cmd2.Flags().Int("fault-ack-timeout", 0, "")

	if hasAnyFaultFlag(cmd2) {
		t.Error("expected false when fault flag not explicitly changed")
	}
}

func TestNewFaultInjectorOrNil_Nil(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("fault-ack-timeout", 0, "")
	// 不传参数

	inj, err := NewFaultInjectorOrNil(cmd)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if inj != nil {
		t.Error("expected nil injector when no fault flags")
	}
}

func TestNewFaultInjectorOrNil_Active(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("fault-ack-timeout", 0, "")
	cmd.Flags().Set("fault-ack-timeout", "500")

	inj, err := NewFaultInjectorOrNil(cmd)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if inj == nil {
		t.Error("expected non-nil injector when fault flag is set")
		return
	}
	if inj.GetAckTimeout() != 500*1_000_000 { // 纳秒
		t.Errorf("expected 500ms, got %v", inj.GetAckTimeout())
	}
}

func TestFaultConfig_HasActiveFaults(t *testing.T) {
	tests := []struct {
		name     string
		config   *FaultConfig
		expected bool
	}{
		{
			name:     "empty config",
			config:   NewFaultConfig(),
			expected: false,
		},
		{
			name: "ack-timeout set",
			config: &FaultConfig{AckTimeout: 1000},
			expected: true,
		},
		{
			name: "reply-drop set",
			config: &FaultConfig{ReplyDrop: true},
			expected: true,
		},
		{
			name: "scenario set",
			config: &FaultConfig{ScenarioName: "network-congestion"},
			expected: true,
		},
		{
			name: "zero values are inactive",
			config: &FaultConfig{AckTimeout: 0, ReplyDelay: 0, RandomDrop: 0},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.HasActiveFaults() != tt.expected {
				t.Errorf("HasActiveFaults() = %v, expected %v", tt.config.HasActiveFaults(), tt.expected)
			}
		})
	}
}

func TestFaultConfig_String(t *testing.T) {
	cfg := &FaultConfig{
		AckTimeout: 1000,
		ReplyDelay: 500,
		ReplyDrop:  true,
		Verror:     true,
	}

	s := cfg.String()

	if s == "" {
		t.Error("expected non-empty string")
	}
	// 验证包含关键字段
}

func TestFaultConfig_ApplyScenario(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		wantErr  bool
	}{
		{"network-congestion", "network-congestion", false},
		{"ack-timeout", "ack-timeout", false},
		{"nack-attack", "nack-attack", false},
		{"seq-disorder", "seq-disorder", false},
		{"version-mismatch", "version-mismatch", false},
		{"data-corruption", "data-corruption", false},
		{"disconnect-loop", "disconnect-loop", false},
		{"verror-disconnect", "verror-disconnect", false},
		{"empty", "", false},
		{"unknown", "unknown-scenario", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewFaultConfig()
			err := cfg.ApplyScenario(tt.scenario)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyScenario(%q) error = %v, wantErr %v", tt.scenario, err, tt.wantErr)
			}
		})
	}
}

func TestScenarios(t *testing.T) {
	// 验证所有预定义场景都能创建有效配置
	scenarios := map[string]*FaultConfig{
		"NetworkCongestion":  NetworkCongestion(),
		"AckTimeout":         AckTimeout(),
		"NackAttack":         NackAttack(),
		"SeqDisorder":        SeqDisorder(),
		"VersionMismatch":    VersionMismatch(),
		"DataCorruption":     DataCorruption(),
		"DisconnectLoop":     DisconnectLoop(),
		"VerrorDisconnect":   VerrorDisconnect(),
	}

	for name, cfg := range scenarios {
		t.Run(name, func(t *testing.T) {
			if cfg == nil {
				t.Errorf("scenario %s returned nil config", name)
				return
			}
			if !cfg.HasActiveFaults() {
				t.Errorf("scenario %s has no active faults", name)
			}
		})
	}
}