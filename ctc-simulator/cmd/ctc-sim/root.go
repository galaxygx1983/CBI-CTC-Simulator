// cmd/ctc-sim/root.go
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ctc-sim",
	Short: "CTC Simulator - gRPC server for CBI communication",
	Long: `CTC (调度集中系统) 模拟器，作为 gRPC 服务端与 CBI 测试器通信。

支持功能:
- 双向帧流通信 (FrameStream)
- 站场状态管理
- 控制命令下发
- 场景脚本执行`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(scenarioCmd)
}