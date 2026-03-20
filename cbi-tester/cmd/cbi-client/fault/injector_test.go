// cmd/cbi-client/fault/injector_test.go
// 故障注入器测试
package fault

import (
	"testing"
	"time"

	"cbi-simulator/internal/protocol"
)

func TestFaultInjector_BeforeSend_WrongVersion(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.WrongVersion = true
	fi := NewFaultInjector(cfg)

	frame := &protocol.Frame{Version: 0x11, Type: protocol.SDCI}
	fi.BeforeSend(frame)

	if frame.Version != 0x10 {
		t.Errorf("expected version 0x10, got 0x%02X", frame.Version)
	}
}

func TestFaultInjector_BeforeSend_ExtraData(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.ExtraData = true
	fi := NewFaultInjector(cfg)

	frame := &protocol.Frame{
		Version:    0x11,
		Type:       protocol.SDCI,
		DataLength: 5,
		Data:       []byte{1, 2, 3, 4, 5},
	}
	fi.BeforeSend(frame)

	if frame.DataLength != 15 { // 5 + 10
		t.Errorf("expected DataLength=15, got %d", frame.DataLength)
	}
	// Data 实际内容不变
	if len(frame.Data) != 5 {
		t.Errorf("expected Data len=5, got %d", len(frame.Data))
	}
}

func TestFaultInjector_BeforeRecv_RandomDrop(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.RandomDrop = 100 // 100% 丢帧
	fi := NewFaultInjector(cfg)

	frame := &protocol.Frame{Type: protocol.ACK}
	result := fi.BeforeRecv(frame)

	if !result.Block {
		t.Error("expected Block=true when RandomDrop=100")
	}
	if fi.Stats().DroppedFrames != 1 {
		t.Errorf("expected DroppedFrames=1, got %d", fi.Stats().DroppedFrames)
	}
}

func TestFaultInjector_BeforeRecv_NackRandom(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.NackRandom = 100
	fi := NewFaultInjector(cfg)

	frame := &protocol.Frame{Type: protocol.ACK}
	result := fi.BeforeRecv(frame)

	if !result.Nack {
		t.Error("expected Nack=true when NackRandom=100")
	}
}

func TestFaultInjector_BeforeRecv_NilInjector(t *testing.T) {
	var fi *FaultInjector = nil

	frame := &protocol.Frame{Type: protocol.ACK}
	result := fi.BeforeRecv(frame)

	if result.Block {
		t.Error("expected Block=false when injector is nil")
	}
	if result.Nack {
		t.Error("expected Nack=false when injector is nil")
	}
}

func TestFaultInjector_ShouldSkipSeq(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.SeqSkip = 2
	fi := NewFaultInjector(cfg)

	// 发1帧，不应跳过
	fi.AfterSend(&protocol.Frame{Type: protocol.SDCI})
	if fi.ShouldSkipSeq() {
		t.Error("should not skip after 1st frame")
	}

	// 发第2帧，应该跳过
	fi.AfterSend(&protocol.Frame{Type: protocol.SDCI})
	if !fi.ShouldSkipSeq() {
		t.Error("should skip after 2nd frame")
	}

	// 再发1帧，不应跳过
	fi.AfterSend(&protocol.Frame{Type: protocol.SDCI})
	if fi.ShouldSkipSeq() {
		t.Error("should not skip after 3rd frame")
	}
}

func TestFaultInjector_AfterRecvCheck_CorruptData(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.CorruptData = true
	fi := NewFaultInjector(cfg)

	originalData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	frame := &protocol.Frame{
		Type: protocol.SDI,
		Data: make([]byte, len(originalData)),
	}
	copy(frame.Data, originalData)

	fi.AfterRecvCheck(frame)

	// 数据应被损坏（至少有一位不同）
	same := true
	for i := range frame.Data {
		if frame.Data[i] != originalData[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("data should be corrupted but was unchanged")
	}
	if fi.Stats().CorruptedFrames != 1 {
		t.Errorf("expected CorruptedFrames=1, got %d", fi.Stats().CorruptedFrames)
	}
}

func TestFaultInjector_AfterRecvCheck_NilInjector(t *testing.T) {
	var fi *FaultInjector = nil

	frame := &protocol.Frame{
		Type: protocol.SDI,
		Data: []byte{0x01, 0x02, 0x03},
	}

	// 不应 panic
	fi.AfterRecvCheck(frame)

	// 数据不变
	if frame.Data[0] != 0x01 || frame.Data[1] != 0x02 || frame.Data[2] != 0x03 {
		t.Error("data should not be changed when injector is nil")
	}
}

func TestFaultInjector_GetAckTimeout(t *testing.T) {
	cfg := NewFaultConfig()
	fi := NewFaultInjector(cfg)

	// 默认值
	if fi.GetAckTimeout() != 490*time.Millisecond {
		t.Errorf("expected 490ms default, got %v", fi.GetAckTimeout())
	}

	// 自定义值
	cfg.AckTimeout = 2000
	fi2 := NewFaultInjector(cfg)
	if fi2.GetAckTimeout() != 2000*time.Millisecond {
		t.Errorf("expected 2000ms, got %v", fi2.GetAckTimeout())
	}
}

func TestFaultInjector_GetReplyDelay(t *testing.T) {
	cfg := NewFaultConfig()
	fi := NewFaultInjector(cfg)

	// 默认值
	if fi.GetReplyDelay() != 10*time.Millisecond {
		t.Errorf("expected 10ms default, got %v", fi.GetReplyDelay())
	}

	// 自定义值
	cfg.ReplyDelay = 500
	fi2 := NewFaultInjector(cfg)
	if fi2.GetReplyDelay() != 500*time.Millisecond {
		t.Errorf("expected 500ms, got %v", fi2.GetReplyDelay())
	}
}

func TestFaultInjector_ShouldReplyDrop(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.ReplyDrop = true
	fi := NewFaultInjector(cfg)

	if !fi.ShouldReplyDrop() {
		t.Error("expected true when ReplyDrop is set")
	}
	if fi.Stats().ReplyDropped != 1 {
		t.Errorf("expected ReplyDropped=1, got %d", fi.Stats().ReplyDropped)
	}
}

func TestFaultInjector_ShouldReplyDrop_NilInjector(t *testing.T) {
	var fi *FaultInjector = nil

	if fi.ShouldReplyDrop() {
		t.Error("expected false when injector is nil")
	}
}

func TestFaultInjector_ShouldBlockDC2(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.BlockDC2 = true
	fi := NewFaultInjector(cfg)

	if !fi.ShouldBlockDC2() {
		t.Error("expected true when BlockDC2 is set")
	}
}

func TestFaultInjector_ShouldSendVerrorOnDC2(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.Verror = true
	fi := NewFaultInjector(cfg)

	if !fi.ShouldSendVerrorOnDC2() {
		t.Error("expected true when Verror is set")
	}
}

func TestFaultInjector_ShouldSendNackAfter(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.NackAfter = 3
	fi := NewFaultInjector(cfg)

	// 前3帧不应发NACK
	for i := 0; i < 3; i++ {
		fi.AfterSend(&protocol.Frame{Type: protocol.SDCI})
	}
	if fi.ShouldSendNackAfter() {
		t.Error("should not send NACK before NackAfter threshold")
	}

	// 第4帧开始应发NACK
	fi.AfterSend(&protocol.Frame{Type: protocol.SDCI})
	if !fi.ShouldSendNackAfter() {
		t.Error("should send NACK after NackAfter threshold")
	}
}

func TestFaultInjector_ShouldDisconnect(t *testing.T) {
	cfg := NewFaultConfig()
	fi := NewFaultInjector(cfg)

	if fi.ShouldDisconnect() {
		t.Error("expected false when DisconnectAfter is 0")
	}

	cfg.DisconnectAfter = 5
	fi2 := NewFaultInjector(cfg)
	if !fi2.ShouldDisconnect() {
		t.Error("expected true when DisconnectAfter > 0")
	}
}

func TestFaultInjector_GetDisconnectAfter(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.DisconnectAfter = 10
	fi := NewFaultInjector(cfg)

	if fi.GetDisconnectAfter() != 10*time.Second {
		t.Errorf("expected 10s, got %v", fi.GetDisconnectAfter())
	}
}

func TestFaultInjector_ShouldReconnectLoop(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.ReconnectLoop = true
	fi := NewFaultInjector(cfg)

	if !fi.ShouldReconnectLoop() {
		t.Error("expected true when ReconnectLoop is set")
	}
}

func TestFaultInjector_ShouldEmptyData(t *testing.T) {
	cfg := NewFaultConfig()
	cfg.EmptyData = true
	fi := NewFaultInjector(cfg)

	if !fi.ShouldEmptyData() {
		t.Error("expected true when EmptyData is set")
	}
}

func TestFaultInjector_RecordDisconnect(t *testing.T) {
	cfg := NewFaultConfig()
	fi := NewFaultInjector(cfg)

	for i := 0; i < 5; i++ {
		fi.RecordDisconnect()
	}

	if fi.Stats().DisconnectTrigger != 5 {
		t.Errorf("expected DisconnectTrigger=5, got %d", fi.Stats().DisconnectTrigger)
	}
}

func TestFaultInjector_RecordDisconnect_NilInjector(t *testing.T) {
	var fi *FaultInjector = nil

	// 不应 panic
	fi.RecordDisconnect()
}

func TestFaultInjector_NilSafeMethods(t *testing.T) {
	var fi *FaultInjector = nil

	// 所有 nil-safe 方法在 injector 为 nil 时不应 panic，并返回默认值
	frame := &protocol.Frame{Type: protocol.SDCI, Version: 0x11}

	// BeforeSend 不应 panic
	fi.BeforeSend(frame)

	// AfterSend 不应 panic
	fi.AfterSend(frame)

	// BeforeRecv 应返回空结果
	result := fi.BeforeRecv(frame)
	if result.Block || result.Nack || result.Verror || result.Corrupt {
		t.Error("BeforeRecv should return empty result when injector is nil")
	}

	// AfterRecvCheck 不应 panic
	fi.AfterRecvCheck(frame)

	// ShouldSkipSeq 应返回 false
	if fi.ShouldSkipSeq() {
		t.Error("ShouldSkipSeq should return false when injector is nil")
	}

	// 其他方法
	if fi.GetAckTimeout() != 490*time.Millisecond {
		t.Error("GetAckTimeout should return default 490ms when injector is nil")
	}
	if fi.GetReplyDelay() != 10*time.Millisecond {
		t.Error("GetReplyDelay should return default 10ms when injector is nil")
	}
	if fi.ShouldBlockDC2() {
		t.Error("ShouldBlockDC2 should return false when injector is nil")
	}
	if fi.ShouldSendVerrorOnDC2() {
		t.Error("ShouldSendVerrorOnDC2 should return false when injector is nil")
	}
	if fi.ShouldReplyDrop() {
		t.Error("ShouldReplyDrop should return false when injector is nil")
	}
	if fi.ShouldSendNackAfter() {
		t.Error("ShouldSendNackAfter should return false when injector is nil")
	}
	if fi.ShouldEmptyData() {
		t.Error("ShouldEmptyData should return false when injector is nil")
	}
	if fi.ShouldDisconnect() {
		t.Error("ShouldDisconnect should return false when injector is nil")
	}
	if fi.GetDisconnectAfter() != 0 {
		t.Error("GetDisconnectAfter should return 0 when injector is nil")
	}
	if fi.ShouldReconnectLoop() {
		t.Error("ShouldReconnectLoop should return false when injector is nil")
	}
	if fi.GetSeqSent() != 0 {
		t.Error("GetSeqSent should return 0 when injector is nil")
	}
}