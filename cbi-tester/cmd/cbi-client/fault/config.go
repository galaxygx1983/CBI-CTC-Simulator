// cmd/cbi-client/fault/config.go
// 故障注入配置 - 支持通过命令行参数模拟各类通信异常
package fault

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FaultConfig 故障注入配置结构体
type FaultConfig struct {
	// === 时序类 ===
	AckTimeout int  // ACK 超时时间（ms），默认 490
	ReplyDelay int  // 所有回复统一延时（ms），0=使用默认值 10
	ReplyDrop  bool // 收到帧后不回复
	RandomDrop int  // 丢帧概率 0-100（%）

	// === 序号类 ===
	SeqSkip     int  // 每发送 N 帧跳过 1 个序号，0=不跳过
	SeqStuck    bool // 序号卡住不递增
	SeqMismatch bool // AckSeq 故意不匹配

	// === 帧类型类 ===
	NackAfter   int  // 收到 N 帧后开始回复 NACK
	NackRandom  int  // N% 概率将正常回复替换为 NACK
	Verror      bool // 收到 DC2 后回复 VERROR
	BlockDC2    bool // 收到 DC2 后不回复 DC3
	WrongVersion bool // 发送帧使用错误版本号 0x10

	// === 数据类 ===
	CorruptData bool // SDI/SDCI 数据随机损坏
	EmptyData   bool // 所有发送帧数据长度为 0
	ExtraData   bool // 数据长度字段与实际不符

	// === 连接类 ===
	DisconnectAfter int  // N 秒后主动断开，0=不断开
	ReconnectLoop  bool // 断开后自动重连循环

	// === 预定义场景 ===
	ScenarioName string // 预定义场景名称
}

// NewFaultConfig 创建默认配置
func NewFaultConfig() *FaultConfig {
	return &FaultConfig{
		AckTimeout: 0, // 0 表示使用默认值 490ms
		ReplyDelay: 0, // 0 表示使用默认值 10ms
	}
}

// RegisterFlags 注册命令行参数
func (fc *FaultConfig) RegisterFlags(cmd *cobra.Command) {
	cmd.Flags().IntVar(&fc.AckTimeout, "fault-ack-timeout", 0, "ACK timeout in ms (0=default 490)")
	cmd.Flags().IntVar(&fc.ReplyDelay, "fault-delay", 0, "Reply delay in ms (0=default 10ms)")
	cmd.Flags().BoolVar(&fc.ReplyDrop, "fault-reply-drop", false, "Drop all replies")
	cmd.Flags().IntVar(&fc.RandomDrop, "fault-random-drop", 0, "Random drop probability 0-100%")
	cmd.Flags().IntVar(&fc.SeqSkip, "fault-seq-skip", 0, "Skip seq every N frames")
	cmd.Flags().BoolVar(&fc.SeqStuck, "fault-seq-stuck", false, "Fix seq at current value")
	cmd.Flags().BoolVar(&fc.SeqMismatch, "fault-seq-mismatch", false, "Offset ackSeq")
	cmd.Flags().IntVar(&fc.NackAfter, "fault-nack-after", 0, "Send NACK after N frames received")
	cmd.Flags().IntVar(&fc.NackRandom, "fault-nack-random", 0, "NACK probability 0-100%")
	cmd.Flags().BoolVar(&fc.Verror, "fault-verror", false, "Reply VERROR on DC2")
	cmd.Flags().BoolVar(&fc.BlockDC2, "fault-block-dc2", false, "Block DC3 reply on DC2")
	cmd.Flags().BoolVar(&fc.WrongVersion, "fault-wrong-version", false, "Use wrong version 0x10")
	cmd.Flags().BoolVar(&fc.CorruptData, "fault-corrupt-data", false, "Corrupt SDI/SDCI data")
	cmd.Flags().BoolVar(&fc.EmptyData, "fault-empty-data", false, "Send frames with empty data")
	cmd.Flags().BoolVar(&fc.ExtraData, "fault-extra-data", false, "Set data length mismatch")
	cmd.Flags().IntVar(&fc.DisconnectAfter, "fault-disconnect-after", 0, "Disconnect after N seconds")
	cmd.Flags().BoolVar(&fc.ReconnectLoop, "fault-reconnect-loop", false, "Auto reconnect after disconnect")
	cmd.Flags().StringVar(&fc.ScenarioName, "fault-scenario", "", "Predefined fault scenario (network-congestion, ack-timeout, nack-attack, seq-disorder, version-mismatch, data-corruption, disconnect-loop, verror-disconnect)")
}

// NewFaultConfigFromCommand 从 cobra 命令解析配置
func NewFaultConfigFromCommand(cmd *cobra.Command) (*FaultConfig, error) {
	cfg := NewFaultConfig()
	fs := cmd.Flags()

	cfg.AckTimeout, _ = fs.GetInt("fault-ack-timeout")
	cfg.ReplyDelay, _ = fs.GetInt("fault-delay")
	cfg.ReplyDrop, _ = fs.GetBool("fault-reply-drop")
	cfg.RandomDrop, _ = fs.GetInt("fault-random-drop")
	cfg.SeqSkip, _ = fs.GetInt("fault-seq-skip")
	cfg.SeqStuck, _ = fs.GetBool("fault-seq-stuck")
	cfg.SeqMismatch, _ = fs.GetBool("fault-seq-mismatch")
	cfg.NackAfter, _ = fs.GetInt("fault-nack-after")
	cfg.NackRandom, _ = fs.GetInt("fault-nack-random")
	cfg.Verror, _ = fs.GetBool("fault-verror")
	cfg.BlockDC2, _ = fs.GetBool("fault-block-dc2")
	cfg.WrongVersion, _ = fs.GetBool("fault-wrong-version")
	cfg.CorruptData, _ = fs.GetBool("fault-corrupt-data")
	cfg.EmptyData, _ = fs.GetBool("fault-empty-data")
	cfg.ExtraData, _ = fs.GetBool("fault-extra-data")
	cfg.DisconnectAfter, _ = fs.GetInt("fault-disconnect-after")
	cfg.ReconnectLoop, _ = fs.GetBool("fault-reconnect-loop")
	cfg.ScenarioName, _ = fs.GetString("fault-scenario")

	// 应用预定义场景
	if cfg.ScenarioName != "" {
		if err := cfg.ApplyScenario(cfg.ScenarioName); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// hasAnyFaultFlag 检测是否有任何 --fault-* 参数被显式传递
// 注意：零值参数（如 --fault-ack-timeout 0）不算"显式传递"，因为 cobra 的 Changed 不会为默认值返回 true
func hasAnyFaultFlag(cmd *cobra.Command) bool {
	var found bool
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if strings.HasPrefix(f.Name, "fault-") && f.Changed {
			found = true
		}
	})
	return found
}

// NewFaultInjectorOrNil 根据是否有 --fault-* 参数决定是否创建 Injector
// 无任何故障参数时返回 nil，保证零侵入
func NewFaultInjectorOrNil(cmd *cobra.Command) (*FaultInjector, error) {
	if !hasAnyFaultFlag(cmd) {
		return nil, nil // 关键：返回 nil 而非空 injector
	}
	cfg, err := NewFaultConfigFromCommand(cmd)
	if err != nil {
		return nil, err
	}
	return NewFaultInjector(cfg), nil
}

// HasActiveFaults 检查是否有任何故障被激活
func (fc *FaultConfig) HasActiveFaults() bool {
	return fc.AckTimeout > 0 || fc.ReplyDelay > 0 || fc.ReplyDrop ||
		fc.RandomDrop > 0 || fc.SeqSkip > 0 || fc.SeqStuck ||
		fc.SeqMismatch || fc.NackAfter > 0 || fc.NackRandom > 0 ||
		fc.Verror || fc.BlockDC2 || fc.WrongVersion ||
		fc.CorruptData || fc.EmptyData || fc.ExtraData ||
		fc.DisconnectAfter > 0 || fc.ReconnectLoop || fc.ScenarioName != ""
}

// String 返回配置的字符串表示
func (fc *FaultConfig) String() string {
	var parts []string

	if fc.AckTimeout > 0 {
		parts = append(parts, fmt.Sprintf("ack-timeout=%dms", fc.AckTimeout))
	}
	if fc.ReplyDelay > 0 {
		parts = append(parts, fmt.Sprintf("delay=%dms", fc.ReplyDelay))
	}
	if fc.ReplyDrop {
		parts = append(parts, "reply-drop")
	}
	if fc.RandomDrop > 0 {
		parts = append(parts, fmt.Sprintf("random-drop=%d%%", fc.RandomDrop))
	}
	if fc.SeqSkip > 0 {
		parts = append(parts, fmt.Sprintf("seq-skip=%d", fc.SeqSkip))
	}
	if fc.SeqStuck {
		parts = append(parts, "seq-stuck")
	}
	if fc.SeqMismatch {
		parts = append(parts, "seq-mismatch")
	}
	if fc.NackAfter > 0 {
		parts = append(parts, fmt.Sprintf("nack-after=%d", fc.NackAfter))
	}
	if fc.NackRandom > 0 {
		parts = append(parts, fmt.Sprintf("nack-random=%d%%", fc.NackRandom))
	}
	if fc.Verror {
		parts = append(parts, "verror")
	}
	if fc.BlockDC2 {
		parts = append(parts, "block-dc2")
	}
	if fc.WrongVersion {
		parts = append(parts, "wrong-version")
	}
	if fc.CorruptData {
		parts = append(parts, "corrupt-data")
	}
	if fc.EmptyData {
		parts = append(parts, "empty-data")
	}
	if fc.ExtraData {
		parts = append(parts, "extra-data")
	}
	if fc.DisconnectAfter > 0 {
		parts = append(parts, fmt.Sprintf("disconnect-after=%ds", fc.DisconnectAfter))
	}
	if fc.ReconnectLoop {
		parts = append(parts, "reconnect-loop")
	}
	if fc.ScenarioName != "" {
		parts = append(parts, fmt.Sprintf("scenario=%s", fc.ScenarioName))
	}

	return strings.Join(parts, ", ")
}

// ApplyScenario 应用预定义场景
func (fc *FaultConfig) ApplyScenario(name string) error {
	switch name {
	case "network-congestion":
		*fc = *NetworkCongestion()
	case "ack-timeout":
		*fc = *AckTimeout()
	case "nack-attack":
		*fc = *NackAttack()
	case "seq-disorder":
		*fc = *SeqDisorder()
	case "version-mismatch":
		*fc = *VersionMismatch()
	case "data-corruption":
		*fc = *DataCorruption()
	case "disconnect-loop":
		*fc = *DisconnectLoop()
	case "verror-disconnect":
		*fc = *VerrorDisconnect()
	case "":
		// 空场景，无操作
	default:
		return fmt.Errorf("unknown scenario: %s", name)
	}
	return nil
}