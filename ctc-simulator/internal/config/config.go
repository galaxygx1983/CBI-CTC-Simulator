// internal/config/config.go
package config

import (
	"time"

	"github.com/spf13/viper"
)

// Config 主配置
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Client  ClientConfig  `mapstructure:"client"`
	Logging LoggingConfig `mapstructure:"logging"`
}

// ServerConfig 服务配置
type ServerConfig struct {
	GRPC GRPCConfig `mapstructure:"grpc"`
}

// ClientConfig 客户端配置
type ClientConfig struct {
	Mode string       `mapstructure:"mode"`
	TCP  TCPConfig    `mapstructure:"tcp"`
}

// TCPConfig TCP 配置
type TCPConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// GRPCConfig gRPC配置
type GRPCConfig struct {
	Address string        `mapstructure:"address"`
	Timeout time.Duration `mapstructure:"timeout"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			GRPC: GRPCConfig{
				Address: ":50051",
				Timeout: 30 * time.Second,
			},
		},
		Client: ClientConfig{
			Mode: "tcp",
			TCP: TCPConfig{
				Host: "127.0.0.1",
				Port: 6000,
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			return nil, err
		}
		if err := viper.Unmarshal(cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}