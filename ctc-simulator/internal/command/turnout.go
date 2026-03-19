// internal/command/turnout.go
package command

import "ctc-simulator/internal/protocol"

// TurnoutOperation 道岔操作类型
type TurnoutOperation byte

const (
	TurnoutOpNormal  TurnoutOperation = 0x01 // 定位操作
	TurnoutOpReverse TurnoutOperation = 0x02 // 反位操作
)

// BuildTurnoutCommand 构建道岔操作命令
func (b *Builder) BuildTurnoutCommand(index uint16, operation TurnoutOperation) *protocol.Frame {
	return b.BuildBCC(index, byte(operation))
}