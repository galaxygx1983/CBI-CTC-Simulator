// internal/config/config.go
package config

import (
	"errors"
	"fmt"
)

// Config 主配置结构
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Protocol ProtocolConfig `mapstructure:"protocol"`
	Station  StationConfig  `mapstructure:"station"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	GRPC     GRPCConfig     `mapstructure:"grpc"` // gRPC配置
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Mode   string       `mapstructure:"mode"` // tcp, serial, pipe
	TCP    TCPConfig    `mapstructure:"tcp"`
	Serial SerialConfig `mapstructure:"serial"`
	Pipe   PipeConfig   `mapstructure:"pipe"`
}

// TCPConfig TCP配置
type TCPConfig struct {
	Port int `mapstructure:"port"`
}

// SerialConfig 串口配置
type SerialConfig struct {
	Port     string `mapstructure:"port"`
	BaudRate int    `mapstructure:"baud_rate"`
	DataBits int    `mapstructure:"data_bits"`
	Parity   string `mapstructure:"parity"`
	StopBits int    `mapstructure:"stop_bits"`
}

// PipeConfig 命名管道配置
type PipeConfig struct {
	Name string `mapstructure:"name"`
}

// GRPCConfig gRPC配置
type GRPCConfig struct {
	Address   string `mapstructure:"address"`    // 服务端地址
	TimeoutMs int    `mapstructure:"timeout_ms"` // 连接超时
	MaxRetry  int    `mapstructure:"max_retry"`  // 最大重试
	Insecure  bool   `mapstructure:"insecure"`   // 是否不安全连接
}

// ProtocolConfig 协议配置
type ProtocolConfig struct {
	Version      uint8 `mapstructure:"version"`
	AckTimeoutMs int   `mapstructure:"ack_timeout_ms"`
	MaxRetry     int   `mapstructure:"max_retry"`
	HeartbeatMs  int   `mapstructure:"heartbeat_ms"`
}

// StationConfig 站场配置
type StationConfig struct {
	CodebitFile  string `mapstructure:"codebit_file"`
	ErrorFile    string `mapstructure:"error_file"`
	InitialState string `mapstructure:"initial_state"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level        string `mapstructure:"level"`
	RunLogPath   string `mapstructure:"run_log"`
	FrameLogPath string `mapstructure:"frame_log"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Mode: "tcp",
			TCP: TCPConfig{
				Port: 8001,
			},
			Serial: SerialConfig{
				Port:     "COM4",
				BaudRate: 19200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			Pipe: PipeConfig{
				Name: "cbi_sim",
			},
		},
		Protocol: ProtocolConfig{
			Version:      0x11,
			AckTimeoutMs: 500,
			MaxRetry:     3,
			HeartbeatMs:  500,
		},
		Station: StationConfig{
			CodebitFile:  "lgxtq.zl",
			ErrorFile:    "Error.sys",
			InitialState: "safe",
		},
		Logging: LoggingConfig{
			Level:        "info",
			RunLogPath:   "logs/run.log",
			FrameLogPath: "logs/ZLEvents",
		},
		GRPC: GRPCConfig{
			Address:   "localhost:50051",
			TimeoutMs: 30000,
			MaxRetry:  3,
			Insecure:  true,
		},
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	validModes := map[string]bool{"tcp": true, "serial": true, "pipe": true, "grpc": true}
	if !validModes[c.Server.Mode] {
		return fmt.Errorf("invalid server mode: %s", c.Server.Mode)
	}

	if c.Server.Mode == "tcp" && (c.Server.TCP.Port < 1 || c.Server.TCP.Port > 65535) {
		return errors.New("invalid TCP port")
	}

	if c.Protocol.Version != 0x11 {
		return errors.New("only protocol version 0x11 is supported")
	}

	return nil
}
