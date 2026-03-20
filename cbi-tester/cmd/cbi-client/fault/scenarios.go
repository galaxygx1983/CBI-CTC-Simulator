// cmd/cbi-client/fault/scenarios.go
// 预定义故障场景
package fault

// NetworkCongestion 网络拥塞：2秒延迟 + 10%丢包
func NetworkCongestion() *FaultConfig {
	return &FaultConfig{
		ReplyDelay: 2000,
		RandomDrop: 10,
	}
}

// AckTimeout ACK超时：超时时间设为2秒
func AckTimeout() *FaultConfig {
	return &FaultConfig{
		AckTimeout: 2000,
	}
}

// NackAttack NACK攻击：收到2帧后开始发送NACK
func NackAttack() *FaultConfig {
	return &FaultConfig{
		NackAfter: 2,
	}
}

// SeqDisorder 序号错乱：每3帧跳过1个序号
func SeqDisorder() *FaultConfig {
	return &FaultConfig{
		SeqSkip: 3,
	}
}

// VersionMismatch 版本不匹配：使用错误版本号
func VersionMismatch() *FaultConfig {
	return &FaultConfig{
		WrongVersion: true,
	}
}

// DataCorruption 数据损坏：SDI/SDCI数据内容随机损坏
func DataCorruption() *FaultConfig {
	return &FaultConfig{
		CorruptData: true,
	}
}

// DisconnectLoop 断连重连循环：5秒后断开，自动重连
func DisconnectLoop() *FaultConfig {
	return &FaultConfig{
		DisconnectAfter: 5,
		ReconnectLoop:   true,
	}
}

// VerrorDisconnect VERROR断开：收到DC2后主动发送VERROR
func VerrorDisconnect() *FaultConfig {
	return &FaultConfig{
		Verror: true,
	}
}

// ReplyDropTest 回复丢弃测试：所有回复被丢弃
func ReplyDropTest() *FaultConfig {
	return &FaultConfig{
		ReplyDrop: true,
	}
}

// BlockDC2Test 阻断DC2测试：收到DC2后不回复DC3
func BlockDC2Test() *FaultConfig {
	return &FaultConfig{
		BlockDC2: true,
	}
}

// EmptyDataTest 空数据测试：所有发送帧数据为空
func EmptyDataTest() *FaultConfig {
	return &FaultConfig{
		EmptyData: true,
	}
}

// RandomNackTest 随机NACK测试：50%概率发NACK
func RandomNackTest() *FaultConfig {
	return &FaultConfig{
		NackRandom: 50,
	}
}