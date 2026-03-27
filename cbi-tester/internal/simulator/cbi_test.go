// internal/simulator/cbi_test.go
package simulator

import (
	"context"
	"testing"
	"time"

	"cbi-simulator/internal/config"
	"cbi-simulator/internal/protocol"
)

func TestNewCBISimulator(t *testing.T) {
	cfg := config.DefaultConfig()
	sim := NewCBISimulator(cfg)
	if sim == nil {
		t.Fatal("NewCBISimulator returned nil")
	}

	// 检查初始状态
	role, mode, ackCount := sim.GetState()
	if role != RoleStateMaster {
		t.Errorf("Expected role 0x%02X, got 0x%02X", RoleStateMaster, role)
	}
	if mode != ControlModeEmergency {
		t.Errorf("Expected mode 0x%02X, got 0x%02X", ControlModeEmergency, mode)
	}
	if ackCount != 0 {
		t.Errorf("Expected ackCount 0, got %d", ackCount)
	}
}

func TestCBISimulatorStartStop(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.TCP.Port = 0 // 随机端口

	sim := NewCBISimulator(cfg)

	ctx := context.Background()
	err := sim.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !sim.IsRunning() {
		t.Error("Should be running")
	}

	time.Sleep(100 * time.Millisecond)

	err = sim.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if sim.IsRunning() {
		t.Error("Should not be running")
	}
}

func TestHandleDC2(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.TCP.Port = 0
	sim := NewCBISimulator(cfg)

	ctx := context.Background()
	err := sim.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sim.Stop()

	// 设置一些状态
	sim.ackCount = 5
	sim.roleState = RoleStateBackup
	sim.controlMode = ControlModeAuto
	sim.sendSeq = 10
	sim.recvAckSeq = 5

	// 模拟收到DC2
	frame := &protocol.Frame{Type: protocol.DC2}
	sim.handleDC2(frame)

	// 等待延时处理
	time.Sleep(20 * time.Millisecond)

	// 验证状态重置
	role, mode, ackCount := sim.GetState()
	if ackCount != 0 {
		t.Errorf("Expected ackCount 0, got %d", ackCount)
	}
	if role != RoleStateMaster {
		t.Errorf("Expected role 0x%02X, got 0x%02X", RoleStateMaster, role)
	}
	if mode != ControlModeEmergency {
		t.Errorf("Expected mode 0x%02X, got 0x%02X", ControlModeEmergency, mode)
	}

	sim.seqMu.Lock()
	if sim.sendSeq != 1 {
		t.Errorf("Expected sendSeq 1, got %d", sim.sendSeq)
	}
	if sim.recvAckSeq != 0 {
		t.Errorf("Expected recvAckSeq 0, got %d", sim.recvAckSeq)
	}
	sim.seqMu.Unlock()
}

func TestSequenceCheck(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.TCP.Port = 0
	sim := NewCBISimulator(cfg)

	ctx := context.Background()
	err := sim.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sim.Stop()

	// 测试正常帧（序号递增）
	sim.seqMu.Lock()
	sim.recvAckSeq = 0
	sim.sendSeq = 1
	sim.seqMu.Unlock()

	frame := &protocol.Frame{
		Type:    protocol.ACK,
		SendSeq: 1,
		AckSeq:  1,
	}
	if !sim.checkAndUpdateSequence(frame) {
		t.Error("Sequence check should pass for normal frame")
	}

	// 验证序号更新
	sim.seqMu.Lock()
	if sim.recvAckSeq != 1 {
		t.Errorf("Expected recvAckSeq 1, got %d", sim.recvAckSeq)
	}
	// sendSeq不应在接收帧时递增，只在发送数据帧后递增
	if sim.sendSeq != 1 {
		t.Errorf("Expected sendSeq 1 (unchanged), got %d", sim.sendSeq)
	}
	sim.seqMu.Unlock()
}

func TestDuplicateFrame(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.TCP.Port = 0
	sim := NewCBISimulator(cfg)

	ctx := context.Background()
	err := sim.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sim.Stop()

	// 设置期望序号
	sim.seqMu.Lock()
	sim.recvAckSeq = 1
	sim.seqMu.Unlock()

	// 测试重复帧（序号相同）
	frame := &protocol.Frame{
		Type:    protocol.TSD,
		SendSeq: 1,
		AckSeq:  1,
	}
	if sim.checkAndUpdateSequence(frame) {
		t.Error("Sequence check should fail for duplicate frame")
	}
}

func TestHandleACK(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.TCP.Port = 0
	sim := NewCBISimulator(cfg)

	ctx := context.Background()
	err := sim.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sim.Stop()

	// 测试ACK计数和响应
	// ACK=2 应该发送SDI
	sim.ackCount = 0
	frame := &protocol.Frame{Type: protocol.ACK}
	sim.handleACK(frame)

	sim.mu.Lock()
	if sim.ackCount != 1 {
		t.Errorf("Expected ackCount 1, got %d", sim.ackCount)
	}
	sim.mu.Unlock()

	// ACK=3 应该发送ACQ
	sim.handleACK(frame)
	sim.mu.Lock()
	if sim.ackCount != 2 {
		t.Errorf("Expected ackCount 2, got %d", sim.ackCount)
	}
	sim.mu.Unlock()
}

func TestGetState(t *testing.T) {
	cfg := config.DefaultConfig()
	sim := NewCBISimulator(cfg)

	// 修改状态
	sim.mu.Lock()
	sim.roleState = RoleStateBackup
	sim.controlMode = ControlModeAuto
	sim.ackCount = 10
	sim.mu.Unlock()

	// 验证GetState返回正确值
	role, mode, ackCount := sim.GetState()
	if role != RoleStateBackup {
		t.Errorf("Expected role 0x%02X, got 0x%02X", RoleStateBackup, role)
	}
	if mode != ControlModeAuto {
		t.Errorf("Expected mode 0x%02X, got 0x%02X", ControlModeAuto, mode)
	}
	if ackCount != 10 {
		t.Errorf("Expected ackCount 10, got %d", ackCount)
	}
}

func TestIsConnected(t *testing.T) {
	cfg := config.DefaultConfig()
	sim := NewCBISimulator(cfg)

	if sim.IsConnected() {
		t.Error("Should not be connected initially")
	}

	sim.mu.Lock()
	sim.connected = true
	sim.mu.Unlock()

	if !sim.IsConnected() {
		t.Error("Should be connected after setting")
	}
}