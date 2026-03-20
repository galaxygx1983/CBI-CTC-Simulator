// cmd/cbi-client/fault/injector.go
// 故障注入器 - 实现各种注入逻辑
package fault

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"cbi-simulator/internal/protocol"
)

// FaultStats 故障统计
type FaultStats struct {
	DroppedFrames     int64 // 丢弃的帧数
	DelayedFrames     int64 // 延时发送的帧数
	CorruptedFrames   int64 // 损坏的帧数
	NackSent          int64 // NACK 发送次数
	SeqSkipped        int64 // 序号跳过次数
	DisconnectTrigger int64 // 主动断连次数
	ReplyDropped      int64 // 阻断回复次数
	VerrorSent        int64 // VERROR 发送次数
}

// String 返回统计的字符串表示
func (s *FaultStats) String() string {
	return fmt.Sprintf("dropped=%d, delayed=%d, corrupted=%d, nack=%d, seq-skip=%d, disconnect=%d, reply-drop=%d, verror=%d",
		s.DroppedFrames, s.DelayedFrames, s.CorruptedFrames, s.NackSent,
		s.SeqSkipped, s.DisconnectTrigger, s.ReplyDropped, s.VerrorSent)
}

// InjectionResult 注入结果
type InjectionResult struct {
	Block   bool          // true = 拦截该帧，不发送/不处理
	Nack    bool          // true = 替换为 NACK
	Verror  bool          // true = 替换为 VERROR
	Delay   time.Duration // 额外的延时
	Corrupt bool          // true = 篡改数据内容
}

// FaultInjector 故障注入器
type FaultInjector struct {
	cfg     *FaultConfig
	stats   *FaultStats
	seqSent int64 // 已发送帧计数（用于 seq-skip / nack-after）
	skipSeq bool  // 下次是否跳过序号
	seqMu   sync.Mutex
	rand    *rand.Rand
}

// NewFaultInjector 创建故障注入器
func NewFaultInjector(cfg *FaultConfig) *FaultInjector {
	return &FaultInjector{
		cfg:   cfg,
		stats: &FaultStats{},
		seqMu: sync.Mutex{},
		rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Stats 获取统计信息
func (fi *FaultInjector) Stats() *FaultStats {
	return fi.stats
}

// Config 获取配置
func (fi *FaultInjector) Config() *FaultConfig {
	return fi.cfg
}

// === BeforeSend：发送帧前篡改（注入点 E）===

// BeforeSend 发送帧前调用 - 处理版本篡改、数据长度篡改
// 注意：nil-safe 设计，injector 为 nil 时调用不会 panic
func (fi *FaultInjector) BeforeSend(frame *protocol.Frame) {
	if fi == nil {
		return
	}

	// WrongVersion: 篡改版本号
	if fi.cfg.WrongVersion {
		frame.Version = 0x10 // 错误版本 0x10，正常为 0x11
		atomic.AddInt64(&fi.stats.VerrorSent, 1)
	}

	// ExtraData: 篡改数据长度字段
	if fi.cfg.ExtraData && len(frame.Data) > 0 {
		frame.DataLength = uint16(len(frame.Data) + 10)
	}
}

// AfterSend 发送后更新序号状态
func (fi *FaultInjector) AfterSend(frame *protocol.Frame) {
	if fi == nil {
		return
	}

	fi.seqMu.Lock()
	defer fi.seqMu.Unlock()

	// SeqSkip: 每 N 帧跳过 1 个序号
	if fi.cfg.SeqSkip > 0 {
		atomic.AddInt64(&fi.seqSent, 1)
		sent := atomic.LoadInt64(&fi.seqSent)
		if sent%int64(fi.cfg.SeqSkip) == 0 {
			fi.skipSeq = true
			atomic.AddInt64(&fi.stats.SeqSkipped, 1)
		}
	} else {
		// 普通计数（用于 nack-after）
		atomic.AddInt64(&fi.seqSent, 1)
	}
}

// ShouldSkipSeq 检查是否应跳过序号（由 FrameHandler.SendDataFrame 调用）
func (fi *FaultInjector) ShouldSkipSeq() bool {
	if fi == nil {
		return false
	}

	fi.seqMu.Lock()
	defer fi.seqMu.Unlock()
	if fi.skipSeq {
		fi.skipSeq = false
		return true
	}
	return false
}

// === BeforeRecv：接收帧后、序号检查前（注入点 A）===

// BeforeRecv 接收帧后、序号检查前调用 - 丢帧 / 随机 NACK
func (fi *FaultInjector) BeforeRecv(frame *protocol.Frame) InjectionResult {
	result := InjectionResult{}

	if fi == nil {
		return result
	}

	// RandomDrop: 随机丢帧
	if fi.cfg.RandomDrop > 0 && fi.rand.Intn(100) < fi.cfg.RandomDrop {
		atomic.AddInt64(&fi.stats.DroppedFrames, 1)
		result.Block = true
		return result
	}

	// NackRandom: 随机 NACK
	if fi.cfg.NackRandom > 0 && fi.rand.Intn(100) < fi.cfg.NackRandom {
		atomic.AddInt64(&fi.stats.NackSent, 1)
		result.Nack = true
		return result
	}

	return result
}

// === AfterRecvCheck：接收帧后、序号检查后（注入点 B）===

// AfterRecvCheck 接收帧后、序号检查后调用 - 数据篡改
func (fi *FaultInjector) AfterRecvCheck(frame *protocol.Frame) {
	if fi == nil {
		return
	}

	if !fi.cfg.CorruptData || len(frame.Data) == 0 {
		return
	}

	// 随机损坏一个字节
	pos := fi.rand.Intn(len(frame.Data))
	frame.Data[pos] ^= 0xFF
	atomic.AddInt64(&fi.stats.CorruptedFrames, 1)
}

// === 回调辅助方法 ===

// ShouldReplyDrop 判断是否阻断回复
func (fi *FaultInjector) ShouldReplyDrop() bool {
	if fi == nil || !fi.cfg.ReplyDrop {
		return false
	}
	atomic.AddInt64(&fi.stats.ReplyDropped, 1)
	return true
}

// ShouldBlockDC2 判断是否阻断 DC3 回复
func (fi *FaultInjector) ShouldBlockDC2() bool {
	if fi == nil {
		return false
	}
	return fi.cfg.BlockDC2
}

// ShouldSendVerrorOnDC2 判断收到 DC2 是否回复 VERROR
func (fi *FaultInjector) ShouldSendVerrorOnDC2() bool {
	if fi == nil {
		return false
	}
	return fi.cfg.Verror
}

// GetReplyDelay 获取回复延时
func (fi *FaultInjector) GetReplyDelay() time.Duration {
	if fi == nil {
		return 10 * time.Millisecond // defaultDelay
	}
	if fi.cfg.ReplyDelay > 0 {
		return time.Duration(fi.cfg.ReplyDelay) * time.Millisecond
	}
	return 10 * time.Millisecond // defaultDelay
}

// GetAckTimeout 获取 ACK 超时时间
func (fi *FaultInjector) GetAckTimeout() time.Duration {
	if fi == nil {
		return 490 * time.Millisecond
	}
	if fi.cfg.AckTimeout > 0 {
		return time.Duration(fi.cfg.AckTimeout) * time.Millisecond
	}
	return 490 * time.Millisecond
}

// ShouldSendNackAfter 判断当前是否应发送 NACK
func (fi *FaultInjector) ShouldSendNackAfter() bool {
	if fi == nil || fi.cfg.NackAfter == 0 {
		return false
	}
	sent := atomic.LoadInt64(&fi.seqSent)
	// NackAfter=N 表示收到 N 帧后开始回复 NACK，所以需要 seqSent > N
	return sent > int64(fi.cfg.NackAfter)
}

// ShouldEmptyData 判断是否返回空数据
func (fi *FaultInjector) ShouldEmptyData() bool {
	if fi == nil {
		return false
	}
	return fi.cfg.EmptyData
}

// ShouldDisconnect 判断是否应主动断开
func (fi *FaultInjector) ShouldDisconnect() bool {
	if fi == nil {
		return false
	}
	return fi.cfg.DisconnectAfter > 0
}

// GetDisconnectAfter 获取断连延时
func (fi *FaultInjector) GetDisconnectAfter() time.Duration {
	if fi == nil {
		return 0
	}
	return time.Duration(fi.cfg.DisconnectAfter) * time.Second
}

// ShouldReconnectLoop 判断是否自动重连
func (fi *FaultInjector) ShouldReconnectLoop() bool {
	if fi == nil {
		return false
	}
	return fi.cfg.ReconnectLoop
}

// RecordDisconnect 记录断连次数
func (fi *FaultInjector) RecordDisconnect() {
	if fi == nil {
		return
	}
	atomic.AddInt64(&fi.stats.DisconnectTrigger, 1)
}

// GetSeqSent 获取已发送帧计数
func (fi *FaultInjector) GetSeqSent() int64 {
	if fi == nil {
		return 0
	}
	return atomic.LoadInt64(&fi.seqSent)
}