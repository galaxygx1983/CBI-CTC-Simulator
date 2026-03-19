// internal/scenario/script.go
package scenario

// ActionType 动作类型
type ActionType string

const (
	ActionTypeBCC     ActionType = "BCC"     // 控制命令
	ActionTypeACQ     ActionType = "ACQ"     // 自律控制请求
	ActionTypeTSQ     ActionType = "TSQ"     // 时间同步请求
	ActionTypeWait    ActionType = "WAIT"    // 等待
	ActionTypeSendRSR ActionType = "SEND_RSR" // 发送状态报告
)

// Action 场景动作
type Action struct {
	DelayMs     int64      `json:"delay_ms"`      // 延迟毫秒
	Type        ActionType `json:"type"`          // 动作类型
	DeviceIndex uint16     `json:"device_index"`  // 设备索引
	Command     string     `json:"command"`       // 命令参数
	Data        []byte     `json:"data,omitempty"` // 原始数据
}

// Script 场景脚本
type Script struct {
	Name          string   `json:"name"`           // 脚本名称
	Description   string   `json:"description"`    // 脚本描述
	InitialStates []byte   `json:"initial_states"` // 初始状态
	Actions       []Action `json:"actions"`        // 动作列表
}