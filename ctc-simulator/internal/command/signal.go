// internal/command/signal.go
package command

import "ctc-simulator/internal/protocol"

// SignalAspect 信号机显示
type SignalAspect byte

const (
	SignalAspectClose SignalAspect = 0x00 // 关闭信号
	SignalAspectOpen  SignalAspect = 0x01 // 开放信号
)

// BuildSignalCommand 构建信号机控制命令
func (b *Builder) BuildSignalCommand(index uint16, aspect SignalAspect) *protocol.Frame {
	return b.BuildBCC(index, byte(aspect))
}