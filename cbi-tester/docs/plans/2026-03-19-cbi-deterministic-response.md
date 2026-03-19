# CBI 确定性响应逻辑实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修改 CBI 模拟器的报文收发流程，实现完全确定性的响应逻辑，移除概率性响应。

**Architecture:** 在现有 CBI 模拟器 (cbi.go) 基础上修改，新增 controlMode 字段，重构 handleFrame 方法，实现基于 ACK 计数器的确定性响应。

**Tech Stack:** Go, tcp transport, protocol frame

---

## 背景需求

### 状态定义
- 主备状态: 0x55=主机, 0xAA=备机, 0xCC=未知
- 控制模式: 0x55=自律控制, 0xAA=非常站控, 0xCC=中间状态

### 帧处理逻辑

| 帧类型 | 处理逻辑 |
|--------|----------|
| DC2 | 计数器置0, 主机=0x55, 非常站控=0xAA, 延时10ms回复DC3, 序号=1 |
| RSR | 回复RSR(主机状态+控制模式) |
| ACK | 根据计数器值确定性响应 |
| ACA | 同意则controlMode=0x55, 延时10ms回复ACK |
| TSD | 延时10ms回复ACK |
| BCC | 延时10ms回复ACK |
| SDIQ | 延时10ms回复SDI |

### ACK 计数器响应表（优先级：SDCI > FIR）

| 计数器值 | 响应帧 | 说明 |
|----------|--------|------|
| 1 | SDI | 第一次ACK回复完整数据 |
| 2 | ACQ | 第二次ACK请求自律控制 |
| 10 | TSQ | 第十次ACK时间同步请求 |
| >10 且 %3==0 | SDCI | 3的倍数优先回复增量数据 |
| >10 且 %5==0 且 %3!=0 | FIR | 5的倍数但非3的倍数回复故障报告 |
| 其他 | ACK（延时10ms） | |

---

## Task 1: 新增 controlMode 字段和初始化

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:41-88`

**Step 1: 在 CBISimulator 结构体中新增 controlMode 字段**

位置: 在 `ackCount` 字段附近添加

```go
// 新增字段 (约第64行附近)
ackCount          int        // 收到 ACK 的次数（收到 DC2 时重置）
controlMode       byte       // 控制模式: 0x55=自律控制, 0xAA=非常站控, 0xCC=中间状态
```

**Step 2: 在 NewCBISimulator 中初始化 controlMode**

位置: 约第113行

```go
// 在 return 语句之前添加
s.controlMode = 0xAA // 默认为非常站控
```

**Step 3: 验证代码可编译**

Run: `cd cbi-tester && go build ./...`
Expected: 编译成功

---

## Task 2: 修改 handleDC2 方法 - 初始化状态

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:316-347`

**Step 1: 修改 handleDC2 方法**

将现有 handleDC2 方法修改为:

```go
// handleDC2 处理 DC2 连接请求
func (s *CBISimulator) handleDC2(frame *protocol.Frame) {
	log.Info("Received DC2, initializing state")

	// 重置状态
	s.mu.Lock()
	s.ackCount = 0
	s.role = RoleMaster      // 主机
	s.controlMode = 0xAA     // 非常站控
	s.mu.Unlock()

	s.seqMu.Lock()
	s.sendSeq = 1
	s.ackSeq = 0
	s.expectedRecvSeq = 0
	s.seqMu.Unlock()

	// 停止重传计时器
	s.seqMu.Lock()
	s.lastSentFrame = nil
	s.waitingForAck = false
	s.seqMu.Unlock()

	if s.retryTimer != nil {
		s.retryTimer.Stop()
	}

	// 延时 10ms 发送 DC3
	time.AfterFunc(10*time.Millisecond, func() {
		log.Info("Sending DC3 after 10ms delay")
		dc3 := &protocol.Frame{
			Type:    protocol.DC3,
			SendSeq: 0x00,
			AckSeq:  0x00,
		}
		s.sendFrame(dc3)
	})
}
```

**Step 2: 验证编译**

Run: `cd cbi-tester && go build ./...`
Expected: 编译成功

---

## Task 3: 重构 handleFrame - 实现确定性响应逻辑

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:236-314`

**Step 1: 完全替换 handleFrame 方法**

删除旧的 probabilisticResponse 相关调用，替换为确定性逻辑:

```go
// handleFrame 处理帧
func (s *CBISimulator) handleFrame(frame *protocol.Frame) {
	// 1. 版本号检查 (已有)
	if frame.Version != protocol.Version {
		log.Warnf("Version mismatch: got 0x%02X, expect 0x%02X", frame.Version, protocol.Version)
		s.sendVERROR()
		return
	}

	// 2. DC2 特殊处理（不受序号控制）
	if frame.Type == protocol.DC2 {
		s.handleDC2(frame)
		return
	}

	// 3. 序号异常处理
	if !s.checkSequence(frame) {
		return
	}

	// 4. 特定帧类型处理
	switch frame.Type {
	case protocol.ACK:
		// ACK 计数递增
		s.ackCount++
		s.handleACKResponse()
		return

	case protocol.RSR:
		// 回复 RSR（根据主备状态和控制模式）
		s.sendRSRForInitialMode()
		return

	case protocol.ACA:
		// 自律控制响应
		if len(frame.Data) > 0 && frame.Data[0] == 0x55 {
			s.mu.Lock()
			s.controlMode = 0x55 // 自律控制
			s.mu.Unlock()
		}
		// 延时 10ms 回复 ACK
		time.AfterFunc(10*time.Millisecond, func() {
			s.sendACK()
		})
		return

	case protocol.TSD:
		// 延时 10ms 回复 ACK
		time.AfterFunc(10*time.Millisecond, func() {
			s.sendACK()
		})
		return

	case protocol.BCC:
		// 延时 10ms 回复 ACK
		time.AfterFunc(10*time.Millisecond, func() {
			s.sendACK()
		})
		return

	case protocol.SDIQ:
		// 延时 10ms 回复 SDI
		time.AfterFunc(10*time.Millisecond, func() {
			s.sendSDI(nil)
		})
		return

	case protocol.NACK:
		s.handleNACK(frame)
		return

	case protocol.VERROR:
		log.Error("Received VERROR, reconnecting...")
		s.sendDC2Reconnect()
		return
	}

	// 触发回调
	if s.onFrameReceived != nil {
		s.onFrameReceived(frame)
	}
}
```

**Step 2: 新增 handleACKResponse 方法**

在 handleFrame 方法后面添加:

```go
// handleACKResponse 根据 ACK 计数器值确定响应
func (s *CBISimulator) handleACKResponse() {
	switch s.ackCount {
	case 1:
		// 第一次 ACK：回复 SDI
		s.sendSDI(nil)

	case 2:
		// 第二次 ACK：回复 ACQ（请求自律控制）
		s.sendACQ()

	case 10:
		// 第十次 ACK：回复 TSQ
		s.sendTSQ()

	default:
		if s.ackCount > 10 {
			// 判断优先级：先3后5
			if s.ackCount%3 == 0 {
				// 3的倍数：回复 SDCI
				s.sendSDCI(s.generateRandomSDCIData())
			} else if s.ackCount%5 == 0 {
				// 5的倍数（非3的倍数）：回复 FIR
				s.sendFIR(s.generateRandomFIRData())
			} else {
				// 其他：延时 10ms 回复 ACK
				time.AfterFunc(10*time.Millisecond, func() {
					s.sendACK()
				})
			}
		} else {
			// 3-9 次 ACK：延时 10ms 回复 ACK
			time.AfterFunc(10*time.Millisecond, func() {
				s.sendACK()
			})
		}
	}
}
```

**Step 3: 验证编译**

Run: `cd cbi-tester && go build ./...`
Expected: 编译成功

---

## Task 4: 移除概率性响应相关代码

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 删除不再使用的方法**

删除以下方法（如果存在）:
- `probabilisticResponse()`
- `probabilisticResponseForACK()`
- `start500msDelay()`

Run: `grep -n "func.*probabilisticResponse\|func.*start500msDelay" cbi-tester/internal/simulator/cbi.go`
查看这些方法的位置，然后删除。

**Step 2: 删除 hasDelay 相关代码**

删除:
- `hasDelay` 字段
- `interruptDelay()` 方法
- 相关延时逻辑

**Step 3: 验证编译**

Run: `cd cbi-tester && go build ./...`
Expected: 编译成功

---

## Task 5: 运行测试验证

**Files:**
- Test: `cbi-tester/internal/simulator/cbi_test.go`

**Step 1: 运行现有测试**

Run: `cd cbi-tester && go test ./internal/simulator/... -v`
Expected: 所有测试通过

**Step 2: 添加新测试验证确定性逻辑**

创建测试文件 `cbi-tester/internal/simulator/deterministic_test.go`:

```go
package simulator

import (
	"testing"
	"time"
)

func TestControlModeInitialization(t *testing.T) {
	sim := NewCBISimulator(DefaultConfig())
	if sim.controlMode != 0xAA {
		t.Errorf("Expected controlMode=0xAA (非常站控), got 0x%02X", sim.controlMode)
	}
}

func TestACKCountResponse_SDI(t *testing.T) {
	sim := NewCBISimulator(DefaultConfig())
	sim.ackCount = 1
	sim.handleACKResponse()
	// 验证 SDI 被调用（通过检查是否发送了帧）
	// 这里可以添加更多的验证逻辑
}

func TestACKCountResponse_ACQ(t *testing.T) {
	sim := NewCBISimulator(DefaultConfig())
	sim.ackCount = 2
	sim.handleACKResponse()
	// 验证 ACQ 被调用
}

func TestACKCountResponse_TSQ(t *testing.T) {
	sim := NewCBISimulator(DefaultConfig())
	sim.ackCount = 10
	sim.handleACKResponse()
	// 验证 TSQ 被调用
}

func TestACKCountResponse_SDCI_Priority(t *testing.T) {
	sim := NewCBISimulator(DefaultConfig())
	// 15 是 3 和 5 的公倍数，应该优先回复 SDCI
	sim.ackCount = 15
	sim.handleACKResponse()
	// 验证 SDCI 被调用（而非 FIR）
}

func TestACKCountResponse_FIR(t *testing.T) {
	sim := NewCBISimulator(DefaultConfig())
	// 20 是 5 的倍数但不是 3 的倍数，应该回复 FIR
	sim.ackCount = 20
	sim.handleACKResponse()
	// 验证 FIR 被调用
}

func TestACKCountResponse_Delay(t *testing.T) {
	sim := NewCBISimulator(DefaultConfig())
	// 3 既不是 1,2,10 也不是大于10的情况，应该延时回复
	sim.ackCount = 3
	sim.handleACKResponse()
	// 验证延时被设置
	if !sim.hasDelay {
		t.Error("Expected delay to be set for ackCount=3")
	}
}
```

**Step 3: 运行测试**

Run: `cd cbi-tester && go test ./internal/simulator/... -v -run TestACK`
Expected: 测试通过

---

## Task 6: 更新 resetConnectionState 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:648-673`

**Step 1: 更新 resetConnectionState**

确保 DC2 重置时正确初始化 controlMode:

```go
// resetConnectionState 重置连接状态（收到 DC2 时调用）
func (s *CBISimulator) resetConnectionState() {
	s.mu.Lock()
	s.firstRSRReceived = false
	s.ackCount = 0
	s.controlMode = 0xAA // 重置为非常站控
	s.autoControlAgreed = false
	s.mu.Unlock()

	// ... 其他代码保持不变
}
```

**Step 2: 验证编译**

Run: `cd cbi-tester && go build ./...`
Expected: 编译成功

---

## Task 7: 最终验证

**Step 1: 运行所有测试**

Run: `cd cbi-tester && go test ./... -v`
Expected: 所有测试通过

**Step 2: 代码检查**

Run: `cd cbi-tester && go vet ./...`
Expected: 无警告

---

## 总结

完成以上任务后，CBI 模拟器将：
1. 正确处理 DC2 连接请求，初始化状态
2. 根据 ACK 计数器实现确定性响应
3. 移除所有概率性响应逻辑
4. 保持版本号和 CRC 错误处理
5. 所有测试通过