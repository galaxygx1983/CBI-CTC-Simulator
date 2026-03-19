// cmd/ctc-sim/start.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ctc-simulator/internal/config"
	"ctc-simulator/internal/grpc"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	startAddress string
	startConfig  string
)

func init() {
	startCmd.Flags().StringVarP(&startAddress, "address", "a", ":50051", "gRPC server address")
	startCmd.Flags().StringVarP(&startConfig, "config", "c", "", "Config file path")

	viper.BindPFlag("server.grpc.address", startCmd.Flags().Lookup("address"))
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the CTC simulator",
	Long: `Start the CTC (调度集中系统) simulator as gRPC server.

Examples:
  ctc-sim start
  ctc-sim start --address :50051
  ctc-sim start --config configs/config.yaml`,
	Run: runStart,
}

func runStart(cmd *cobra.Command, args []string) {
	// 加载配置
	cfg := loadConfig()

	// 创建服务端
	server := grpc.NewServer(cfg.Server.GRPC.Address)

	// 启动服务
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("CTC Simulator started\n")
	fmt.Printf("Listening on %s\n", cfg.Server.GRPC.Address)
	fmt.Println("Press Ctrl+C to stop")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	if err := server.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping server: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("CTC Simulator stopped")
}

func loadConfig() *config.Config {
	// 设置默认配置
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("/etc/ctc-sim")

	// 从配置文件加载
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("No config file found, using defaults")
	}

	cfg := config.DefaultConfig()

	// 从viper覆盖配置
	if address := viper.GetString("server.grpc.address"); address != "" {
		cfg.Server.GRPC.Address = address
	}

	return cfg
}