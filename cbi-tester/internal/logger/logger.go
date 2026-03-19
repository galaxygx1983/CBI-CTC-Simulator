// internal/logger/logger.go
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// 帧类型常量
const (
	DC2    = 0x12
	DC3    = 0x13
	ACK    = 0x06
	NACK   = 0x15
	VERROR = 0x10
	FIR    = 0x65
	SDIQ   = 0x6A
	SDI    = 0x85
	SDCI   = 0x8A
	BCC    = 0x95
	TSQ    = 0x9A
	TSD    = 0xA5
	RSR    = 0xAA
	ACQ    = 0x75
	ACA    = 0x7A
)

// frameTypeNames 帧类型名称映射
var frameTypeNames = map[byte]string{
	DC2:    "DC2",
	DC3:    "DC3",
	ACK:    "ACK",
	NACK:   "NACK",
	VERROR: "VERROR",
	FIR:    "FIR",
	SDIQ:   "SDIQ",
	SDI:    "SDI",
	SDCI:   "SDCI",
	BCC:    "BCC",
	TSQ:    "TSQ",
	TSD:    "TSD",
	RSR:    "RSR",
	ACQ:    "ACQ",
	ACA:    "ACA",
}

// Config 日志配置
type Config struct {
	RunLogPath   string
	FrameLogPath string
}

// Logger 双日志记录器
type Logger struct {
	runFile   *os.File
	frameFile *os.File
	mu        sync.Mutex // 用于同步日志写入
}

// NewLogger 创建日志记录器
func NewLogger(cfg *Config) (*Logger, error) {
	if dir := filepath.Dir(cfg.RunLogPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create run log dir: %w", err)
		}
	}
	if dir := filepath.Dir(cfg.FrameLogPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create frame log dir: %w", err)
		}
	}

	var runFile *os.File
	if cfg.RunLogPath != "" {
		var err error
		runFile, err = os.OpenFile(cfg.RunLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open run log: %w", err)
		}
	}

	frameFile, err := os.OpenFile(cfg.FrameLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		if runFile != nil {
			runFile.Close()
		}
		return nil, fmt.Errorf("open frame log: %w", err)
	}

	return &Logger{runFile: runFile, frameFile: frameFile}, nil
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	var errs []error
	if l.runFile != nil {
		if err := l.runFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if l.frameFile != nil {
		if err := l.frameFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}

// LogFrameSend 记录发送帧
func (l *Logger) LogFrameSend(frameType byte, data []byte) {
	l.logFrame(frameType, data, ">>")
}

// LogFrameRecv 记录接收帧
func (l *Logger) LogFrameRecv(frameType byte, data []byte) {
	l.logFrame(frameType, data, "<<")
}

func (l *Logger) logFrame(frameType byte, data []byte, direction string) {
	if l.frameFile == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	timestamp := time.Now().Format("15:04:05.000")
	typeName := getFrameTypeName(frameType)
	hexData := formatHexDataFull(data)
	line := fmt.Sprintf("%s %s[%-5s]%s\n", timestamp, direction, typeName, hexData)
	l.frameFile.WriteString(line)
}

// LogRun 记录运行日志
func (l *Logger) LogRun(level log.Level, msg string) {
	if l.runFile == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	levelStr := strings.ToUpper(level.String())
	line := fmt.Sprintf("[%s] [%s] %s\n", timestamp, levelStr, msg)
	l.runFile.WriteString(line)
}

func getFrameTypeName(frameType byte) string {
	if name, ok := frameTypeNames[frameType]; ok {
		return name
	}
	return fmt.Sprintf("0x%02X", frameType)
}

func formatHexDataFull(data []byte) string {
	var sb strings.Builder
	for i, b := range data {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(fmt.Sprintf("%02X", b))
	}
	return sb.String()
}

// LogError 记录错误日志
func (l *Logger) LogError(msg string) {
	if l.frameFile == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	timestamp := time.Now().Format("15:04:05.000")
	line := fmt.Sprintf("%s Er%s\n", timestamp, msg)
	l.frameFile.WriteString(line)
}

// LogErrorNoAck 记录 ACK 超时错误
func (l *Logger) LogErrorNoAck() {
	l.LogError("未收到 ACK")
}
