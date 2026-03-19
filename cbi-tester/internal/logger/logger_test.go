// internal/logger/logger_test.go
package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLogger(t *testing.T) {
	cfg := &Config{
		RunLogPath:   filepath.Join(t.TempDir(), "run.log"),
		FrameLogPath: filepath.Join(t.TempDir(), "frame.log"),
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	if logger.runFile == nil {
		t.Error("Run log file should not be nil")
	}
	if logger.frameFile == nil {
		t.Error("Frame log file should not be nil")
	}
}

func TestLogFrame(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	cfg := &Config{
		RunLogPath:   filepath.Join(tmpDir, "run.log"),
		FrameLogPath: filepath.Join(tmpDir, "frame.log"),
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	// 测试发送帧日志
	logger.LogFrameSend(SDCI, []byte{0x7D, 0x04, 0x11, 0x01, 0x00, 0x8A, 0x03, 0x00, 0x00, 0x25, 0x01, 0x7E})

	// 测试接收帧日志
	logger.LogFrameRecv(DC2, []byte{0x7D, 0x04, 0x11, 0x00, 0x00, 0x12, 0x66, 0xD6, 0x7E})

	// 读取并验证帧日志
	content, err := os.ReadFile(cfg.FrameLogPath)
	if err != nil {
		t.Fatalf("Read frame log failed: %v", err)
	}

	if len(content) == 0 {
		t.Error("Frame log should not be empty")
	}
}

func TestFrameLogFormat(t *testing.T) {
	// 验证日志格式: 时间 >>[帧类型] 十六进制数据
	tmpDir := t.TempDir()

	cfg := &Config{
		RunLogPath:   filepath.Join(tmpDir, "run.log"),
		FrameLogPath: filepath.Join(tmpDir, "frame.log"),
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	logger.LogFrameSend(SDCI, []byte{0x7D, 0x04, 0x11, 0x7E})

	content, _ := os.ReadFile(cfg.FrameLogPath)
	// 格式: HH:MM:SS >>[DCI  ]7D 04 11 7E
	// 验证包含 >>[SDCI]
	if !bytes.Contains(content, []byte(">>[SDCI")) {
		t.Error("Frame log should contain '>>[SDCI'")
	}
}