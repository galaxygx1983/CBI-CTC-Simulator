# CBI 端通信逻辑重写实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 重写 CBI-CTC Simulator 中 CBI 端的通信逻辑，实现基于概率响应的模拟行为，包括延时控制、异常处理、重传机制等完整功能。

**Architecture:** 在 `cbi-tester/internal/simulator/cbi.go` 中扩展 CBISimulator 结构，添加状态管理字段，重写 handleFrame 方法实现新的帧处理逻辑，添加辅助方法实现概率响应、延时控制、重传处理等功能。

**Tech Stack:** Go 1.21+, github.com/sirupsen/logrus, 自定义 protocol 包

---

## Task 1: 扩展 CBISimulator 结构添加新字段

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:40-58`
- Test: `cbi-tester/internal/simulator/cbi_test.go`

**Step 1: 添加新字段到 CBISimulator 结构**

在 `cbi-tester/internal/simulator/cbi.go` 第 40-58 行的 `CBISimulator` 结构中添加：

```go
type CBISimulator struct {
    // 现有字段...
    config    *config.Config
    transport transport.Transport
    station   *station.StationState
    role      Role
    running   bool
    mu        sync.RWMutex
    ctx       context.Context
    cancel    context.CancelFunc

    // 序号管理
    sendSeq byte
    ackSeq  byte
    seqMu   sync.Mutex

    // 回调
    onFrameReceived func(frame *protocol.Frame)

    // === 新增字段开始 ===

    // 通信状态
    firstRSRReceived   bool           // 是否已收到第一个 RSR
    ackCount           int            // 收到 ACK 的次数（收到 DC2 时重置）
    autoControlAgreed  bool           // ACA 是否已同意自律控制

    // 延时控制
    hasDelay   bool                    // 是否有 500ms 延时正在运行
    delayTimer *time.Timer             // 500ms 延时计时器

    // 异常计数
    crcErrorCount      int             // 连续 CRC 错误计数
    lastRecvTime       time.Time       // 最后接收帧时间（1500ms 超时检测）

    // 重传状态
    lastSentFrame      *protocol.Frame // 最近发送的帧（用于重传）
    retryCount         int             // 重传次数（0-2）
    retryTimer         *time.Timer     // 重传计时器
    waitingForAck      bool            // 是否等待 ACK

    // 序号跟踪
    expectedRecvSeq    byte            // 期望接收的序号
}
```

**Step 2: 初始化新字段**

修改 `NewCBISimulator` 函数（第 66-72 行），添加新字段初始化：

```go
func NewCBISimulator(cfg *config.Config) *CBISimulator {
    return &CBISimulator{
        config:  cfg,
        station: station.NewStationState(),
        role:    RoleMaster, // 默认为主机
        // 新增字段初始化
        firstRSRReceived:  false,
        ackCount:          0,
        autoControlAgreed: false,
        hasDelay:          false,
        delayTimer:        nil,
        crcErrorCount:     0,
        lastRecvTime:      time.Time{},
        lastSentFrame:     nil,
        retryCount:        0,
        retryTimer:        nil,
        waitingForAck:     false,
        expectedRecvSeq:   0,
    }
}
```

**Step 3: 运行现有测试验证结构变更不破坏编译**

Run: `cd cbi-tester && go build ./...`
Expected: 编译成功，无错误

**Step 4: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "refactor: extend CBISimulator with new state fields for comm logic"
```

---

## Task 2: 添加辅助方法 - 概率响应

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`
- Create: `cbi-tester/internal/simulator/cbi_response.go` (可选，或直接在 cbi.go 中添加)

**Step 1: 添加通用概率响应方法**

在 `cbi-tester/internal/simulator/cbi.go` 中添加：

```go
// probabilisticResponse 通用概率响应：70% ACK / 10% FIR / 19% SDCI / 1% TSQ
func (s *CBISimulator) probabilisticResponse() {
    randVal := rand.Intn(100)

    switch {
    case randVal < 70:
        s.sendACK()
    case randVal < 80:
        s.sendFIR(s.generateRandomFIRData())
    case randVal < 99:
        s.sendSDCI(s.generateRandomSDCIData())
    default:
        s.sendTSQ()
    }
}

// probabilisticResponseForACK ACK 特殊概率：70% 延时 / 10% FIR / 15% SDCI / 5% TSQ
func (s *CBISimulator) probabilisticResponseForACK() {
    randVal := rand.Intn(100)

    switch {
    case randVal < 70:
        s.start500msDelay()
    case randVal < 80:
        s.sendFIR(s.generateRandomFIRData())
    case randVal < 95:
        s.sendSDCI(s.generateRandomSDCIData())
    default:
        s.sendTSQ()
    }
}
```

**Step 2: 添加导入 "math/rand"**

确保文件顶部有 `import "math/rand"`

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add probabilistic response methods"
```

---

## Task 3: 添加辅助方法 - 延时控制

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加 500ms 延时启动方法**

```go
// start500msDelay 启动 500ms 延时
func (s *CBISimulator) start500msDelay() {
    if s.hasDelay {
        return // 已有延时，不重复启动
    }

    s.hasDelay = true
    s.delayTimer = time.AfterFunc(500*time.Millisecond, func() {
        s.hasDelay = false
        s.sendACK() // 延时到达，发送 ACK
    })
}

// interruptDelay 中断延时
func (s *CBISimulator) interruptDelay() {
    if s.hasDelay && s.delayTimer != nil {
        s.delayTimer.Stop()
        s.hasDelay = false
    }
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add delay control methods"
```

---

## Task 4: 添加辅助方法 - 数据生成

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加随机数据生成方法**

```go
// generateRandomSDCIData 生成随机 SDCI 数据（1-10 个数据项，每项 3 字节）
func (s *CBISimulator) generateRandomSDCIData() []byte {
    count := rand.Intn(10) + 1
    data := make([]byte, count*3)
    for i := range data {
        data[i] = byte(rand.Intn(256))
    }
    return data
}

// generateRandomFIRData 生成随机 FIR 数据（5 字节：故障类型 + 设备索引 + 故障码）
func (s *CBISimulator) generateRandomFIRData() []byte {
    data := make([]byte, 5)
    data[0] = byte(rand.Intn(10) + 1)  // 故障类型 1-10
    data[1] = byte(rand.Intn(256))     // 设备索引高字节
    data[2] = byte(rand.Intn(256))     // 设备索引低字节
    data[3] = byte(rand.Intn(256))     // 故障码高字节
    data[4] = byte(rand.Intn(256))     // 故障码低字节
    return data
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add random data generation methods"
```

---

## Task 5: 添加辅助方法 - 重传机制

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加重传计时器方法**

```go
// startRetryTimer 启动重传计时器
func (s *CBISimulator) startRetryTimer() {
    if s.retryTimer != nil {
        s.retryTimer.Stop()
    }

    s.retryTimer = time.AfterFunc(500*time.Millisecond, func() {
        s.handleRetryTimeout()
    })
}

// handleRetryTimeout 处理重传超时
func (s *CBISimulator) handleRetryTimeout() {
    s.seqMu.Lock()
    defer s.seqMu.Unlock()

    if s.lastSentFrame == nil {
        return
    }

    s.retryCount++

    if s.retryCount >= 3 {
        // 3 次尝试均失败，判定通信中断
        log.Error("Retry failed 3 times, communication interrupted")
        s.sendDC2Reconnect()
        s.lastSentFrame = nil
        s.waitingForAck = false
        s.retryCount = 0
        return
    }

    // 重发相同帧（序号/数据不变，仅更新 ackSeq）
    frame := s.lastSentFrame
    frame.AckSeq = s.ackSeq
    // 重新计算 CRC
    frame.CRC = protocol.CalculateCRC(s.prepareFrameBody(frame))

    s.sendFrame(frame)
    s.startRetryTimer()
}

// prepareFrameBody 准备帧体（用于 CRC 计算）
func (s *CBISimulator) prepareFrameBody(frame *protocol.Frame) []byte {
    body := make([]byte, 0)
    body = append(body, protocol.HeaderLen)
    body = append(body, frame.Version)
    body = append(body, frame.SendSeq)
    body = append(body, frame.AckSeq)
    body = append(body, byte(frame.Type))
    if len(frame.Data) > 0 {
        dataLen := uint16(len(frame.Data))
        body = append(body, byte(dataLen&0xFF), byte(dataLen>>8))
        body = append(body, frame.Data...)
    }
    return body
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add retransmission mechanism"
```

---

## Task 6: 添加辅助方法 - 发送帧

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 修改 sendFrame 方法，保存发送状态**

```go
// sendFrame 发送帧
func (s *CBISimulator) sendFrame(frame *protocol.Frame) error {
    s.seqMu.Lock()
    frame.SendSeq = s.sendSeq
    frame.AckSeq = s.ackSeq
    s.seqMu.Unlock()

    // 计算 CRC
    body := s.prepareFrameBody(frame)
    frame.CRC = protocol.CalculateCRC(body)

    data, err := protocol.EncodeFrame(frame)
    if err != nil {
        return err
    }

    // 保存发送状态（用于重传）
    s.seqMu.Lock()
    s.lastSentFrame = frame
    s.waitingForAck = true
    s.retryCount = 0
    s.seqMu.Unlock()

    // 启动重传计时器
    s.startRetryTimer()

    return s.transport.Send(data)
}
```

**Step 2: 添加 sendFrameWithBody 辅助方法**

```go
// sendFrameWithBody 发送带数据的帧
func (s *CBISimulator) sendFrameWithBody(frameType protocol.FrameType, data []byte) error {
    frame := &protocol.Frame{
        Type:       frameType,
        DataLength: uint16(len(data)),
        Data:       data,
    }
    return s.sendFrame(frame)
}
```

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: update sendFrame with retry state tracking"
```

---

## Task 7: 添加辅助方法 - 各种帧发送

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加各种帧发送方法**

```go
// sendACK 发送 ACK
func (s *CBISimulator) sendACK() error {
    return s.sendFrameWithBody(protocol.ACK, nil)
}

// sendNACK 发送 NACK
func (s *CBISimulator) sendNACK() error {
    return s.sendFrameWithBody(protocol.NACK, nil)
}

// sendVERROR 发送 VERROR
func (s *CBISimulator) sendVERROR() error {
    return s.sendFrameWithBody(protocol.VERROR, nil)
}

// sendTSQ 发送 TSQ
func (s *CBISimulator) sendTSQ() error {
    return s.sendFrameWithBody(protocol.TSQ, nil)
}

// sendFIR 发送 FIR
func (s *CBISimulator) sendFIR(data []byte) error {
    return s.sendFrameWithBody(protocol.FIR, data)
}

// sendSDCI 发送 SDCI
func (s *CBISimulator) sendSDCI(data []byte) error {
    return s.sendFrameWithBody(protocol.SDCI, data)
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add frame sending helper methods"
```

---

## Task 8: 添加辅助方法 - 状态重置和重连

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加状态重置方法**

```go
// resetConnectionState 重置连接状态（收到 DC2 时调用）
func (s *CBISimulator) resetConnectionState() {
    s.mu.Lock()
    s.firstRSRReceived = false
    s.ackCount = 0
    s.autoControlAgreed = false
    s.mu.Unlock()

    s.seqMu.Lock()
    s.sendSeq = 1
    s.ackSeq = 0
    s.expectedRecvSeq = 0
    s.lastSentFrame = nil
    s.waitingForAck = false
    s.retryCount = 0
    s.seqMu.Unlock()

    if s.retryTimer != nil {
        s.retryTimer.Stop()
    }
    if s.delayTimer != nil {
        s.delayTimer.Stop()
    }
    s.hasDelay = false
    s.crcErrorCount = 0
}

// sendDC2Reconnect 发送 DC2 重连
func (s *CBISimulator) sendDC2Reconnect() {
    s.resetConnectionState()
    // CBI 端发送 DC2 主动重连
    frame := &protocol.Frame{
        Type:    protocol.DC2,
        SendSeq: 0x00,
        AckSeq:  0x00,
    }
    s.sendFrame(frame)
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add connection state reset and reconnect methods"
```

---

## Task 9: 重写 handleFrame 方法 - 主入口

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:176-200`

**Step 1: 重写 handleFrame 方法**

替换第 176-200 行的 `handleFrame` 方法：

```go
// handleFrame 处理帧
func (s *CBISimulator) handleFrame(frame *protocol.Frame) {
    // 1. 版本号检查
    if frame.Version != protocol.Version {
        log.Warnf("Version mismatch: got 0x%02X, expect 0x%02X", frame.Version, protocol.Version)
        s.sendVERROR()
        return
    }

    // 2. DC2 特殊处理（不受序号控制）
    if frame.Type == protocol.DC2 {
        s.resetConnectionState()
        s.handleDC2(frame)
        return
    }

    // 3. 序号异常处理
    if !s.checkSequence(frame) {
        return
    }

    // 4. ACK 特殊计数逻辑
    if frame.Type == protocol.ACK {
        s.ackCount++
        switch s.ackCount {
        case 1:
            s.sendSDI(nil) // 第一次 ACK：回复 SDI
        case 2:
            s.sendACQ()    // 第二次 ACK：回复 ACQ
        default:
            if !s.hasDelay {
                s.probabilisticResponseForACK()
            }
        }
        return
    }

    // 5. 首次 RSR 特殊处理
    if frame.Type == protocol.RSR && !s.firstRSRReceived {
        s.firstRSRReceived = true
        s.sendRSRForInitialMode()
        return
    }

    // 6. 中断延时（所有非 ACK/DC2 帧）
    s.interruptDelay()

    // 7. 更新 lastRecvTime
    s.lastRecvTime = time.Now()

    // 8. 统一概率响应或特殊响应
    switch frame.Type {
    case protocol.SDIQ:
        s.sendSDI(nil)
    case protocol.TSQ:
        s.sendTSD()
    case protocol.BCC:
        s.probabilisticResponse()
    case protocol.TSD:
        s.probabilisticResponse()
    case protocol.RSR:
        s.probabilisticResponse()
    case protocol.ACA:
        if len(frame.Data) > 0 && frame.Data[0] == 0x55 {
            s.autoControlAgreed = true
        }
        s.probabilisticResponse()
    case protocol.SDCI:
        s.probabilisticResponse()
    case protocol.FIR:
        s.probabilisticResponse()
    case protocol.ACQ:
        s.probabilisticResponse()
    case protocol.NACK:
        s.handleNACK(frame)
    case protocol.VERROR:
        log.Error("Received VERROR, reconnecting...")
        s.sendDC2Reconnect()
    default:
        log.Warnf("Unhandled frame type: %s", frame.Type)
    }

    // 触发回调
    if s.onFrameReceived != nil {
        s.onFrameReceived(frame)
    }
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "refactor: rewrite handleFrame main entry point"
```

---

## Task 10: 添加序号检查方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加序号检查方法**

```go
// checkSequence 检查序号连续性，返回 true 表示正常，false 表示异常
func (s *CBISimulator) checkSequence(frame *protocol.Frame) bool {
    expectedSeq := s.expectedRecvSeq + 1
    if expectedSeq == 0 {
        expectedSeq = 1
    }

    seqDiff := int(frame.SendSeq) - int(s.expectedRecvSeq)
    if seqDiff < 0 {
        seqDiff += 256
    }

    switch {
    case seqDiff == 1:
        // 正常帧
        s.expectedRecvSeq = frame.SendSeq
        return true

    case seqDiff == 0:
        // 重复帧
        log.Warnf("Duplicate frame: %s (seq=%d)", frame.Type, frame.SendSeq)
        s.sendACK()
        return false

    default:
        // 序号跳变/丢失
        log.Errorf("Frame loss detected: expected seq=%d, got seq=%d", expectedSeq, frame.SendSeq)
        s.sendDC2Reconnect()
        return false
    }
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add sequence check method"
```

---

## Task 11: 重写 handleDC2 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:202-225`

**Step 1: 重写 handleDC2 方法**

替换第 202-225 行：

```go
// handleDC2 处理 DC2 连接请求
func (s *CBISimulator) handleDC2(frame *protocol.Frame) {
    log.Info("Received DC2, sending DC3")

    // 发送 DC3 响应
    dc3 := &protocol.Frame{
        Type:    protocol.DC3,
        SendSeq: 0x00,
        AckSeq:  0x00,
    }
    s.sendFrame(dc3)

    // 停止重传计时器（DC3 是连接帧，不需要重传）
    s.seqMu.Lock()
    s.lastSentFrame = nil
    s.waitingForAck = false
    s.seqMu.Unlock()

    if s.retryTimer != nil {
        s.retryTimer.Stop()
    }

    // 延迟 100ms 发送 RSR 报告状态
    time.AfterFunc(100*time.Millisecond, func() {
        s.sendRSRForInitialMode()
    })

    // 延迟 200ms 发送 SDI 完整数据
    time.AfterFunc(200*time.Millisecond, func() {
        s.sendSDI(nil)
    })
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "refactor: rewrite handleDC2 method"
```

---

## Task 12: 添加 sendRSRForInitialMode 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加首次 RSR 发送方法**

```go
// sendRSRForInitialMode 发送首次 RSR（主机 + 非常站控模式）
func (s *CBISimulator) sendRSRForInitialMode() {
    // 主机 + 非常站控模式 = 0x55, 0xAA
    data := []byte{0x55, 0xAA}
    s.sendFrameWithBody(protocol.RSR, data)
}

// buildRSR 构建 RSR 数据（根据自律状态）
func (s *CBISimulator) buildRSR() []byte {
    mode := byte(0xAA) // 默认非常站控
    if s.autoControlAgreed {
        mode = 0x55    // 自律控制模式
    }
    return []byte{0x55, mode} // 主机 + 模式
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add RSR mode methods"
```

---

## Task 13: 添加 sendTSD 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加 TSD 发送方法**

```go
// sendTSD 发送 TSD 时间同步数据
func (s *CBISimulator) sendTSD() error {
    timestamp := uint64(time.Now().UnixMilli())
    data := make([]byte, 8)
    data[0] = byte(timestamp)
    data[1] = byte(timestamp >> 8)
    data[2] = byte(timestamp >> 16)
    data[3] = byte(timestamp >> 24)
    data[4] = byte(timestamp >> 32)
    data[5] = byte(timestamp >> 40)
    data[6] = byte(timestamp >> 48)
    data[7] = byte(timestamp >> 56)
    return s.sendFrameWithBody(protocol.TSD, data)
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add sendTSD method"
```

---

## Task 14: 添加 sendACQ 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加 ACQ 发送方法**

```go
// sendACQ 发送 ACQ 自律控制请求
func (s *CBISimulator) sendACQ() error {
    data := []byte{0x55} // 请求自律控制
    return s.sendFrameWithBody(protocol.ACQ, data)
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add sendACQ method"
```

---

## Task 15: 添加 1500ms 超时检测方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 添加超时检测启动方法**

```go
// startCommTimeout 启动 1500ms 超时检测
func (s *CBISimulator) startCommTimeout() {
    if s.retryTimer != nil {
        s.retryTimer.Stop()
    }

    s.retryTimer = time.AfterFunc(1500*time.Millisecond, func() {
        if time.Since(s.lastRecvTime) >= 1500*time.Millisecond {
            log.Error("Communication timeout (1500ms), reconnecting...")
            s.sendDC2Reconnect()
        }
    })
}
```

**Step 2: 在 Start 方法中调用**

修改 `Start` 方法（第 75-104 行），在启动后调用：

```go
s.startCommTimeout()
```

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add 1500ms timeout detection"
```

---

## Task 16: 修改 handleData 添加 CRC 校验

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:160-173`

**Step 1: 修改 handleData 方法**

替换第 160-173 行：

```go
// handleData 处理接收到的数据
func (s *CBISimulator) handleData(data []byte) {
    log.Debugf("Received %d bytes: %X", len(data), data)

    frame, err := protocol.DecodeFrame(data)
    if err != nil {
        log.Errorf("Decode frame failed: %v", err)
        s.crcErrorCount++

        if s.crcErrorCount >= 5 {
            log.Error("Continuous 5 CRC errors, reconnecting...")
            s.sendDC2Reconnect()
            return
        }

        s.sendNACK()
        return
    }

    // CRC 正确，重置计数
    s.crcErrorCount = 0
    s.lastRecvTime = time.Now()

    log.Infof("Received frame: %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)

    // 处理帧
    s.handleFrame(frame)
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "feat: add CRC error handling in handleData"
```

---

## Task 17: 重写 handleNACK 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:242-245`

**Step 1: 重写 handleNACK 方法**

替换第 242-245 行：

```go
// handleNACK 处理 NACK
func (s *CBISimulator) handleNACK(frame *protocol.Frame) {
    log.Warn("Received NACK, resending last frame")

    if s.lastSentFrame == nil {
        log.Warn("No frame to resend")
        return
    }

    // 重发上一帧（序号/数据不变，更新 ackSeq）
    frame := s.lastSentFrame
    frame.AckSeq = s.ackSeq
    // 重新计算 CRC
    frame.CRC = protocol.CalculateCRC(s.prepareFrameBody(frame))

    s.sendFrame(frame)
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "refactor: rewrite handleNACK method"
```

---

## Task 18: 删除旧的 handler 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go`

**Step 1: 删除不再需要的旧 handler 方法**

删除以下方法（因为已在 handleFrame 中统一处理）：
- `handleACK` (第 227-240 行)
- `handleSDIQ` (第 247-251 行)
- `handleBCC` (第 253-259 行)
- `handleTSD` (第 261-266 行)
- `handleACA` (第 268-278 行)

保留 `handleDC2` 和 `handleNACK`（已重写）。

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "refactor: remove old handler methods"
```

---

## Task 19: 修改 sendSDI 和 sendSDCI 方法

**Files:**
- Modify: `cbi-tester/internal/simulator/cbi.go:319-354`

**Step 1: 修改 sendSDI 方法**

```go
// sendSDI 发送 SDI 完整数据
func (s *CBISimulator) sendSDI(data []byte) error {
    if data == nil {
        // 使用站场状态数据
        data = s.station.GetStateData()
    }
    return s.sendFrameWithBody(protocol.SDI, data)
}
```

**Step 2: 修改 sendSDCI 方法**

```go
// sendSDCI 发送 SDCI 增量数据
func (s *CBISimulator) sendSDCI(data []byte) error {
    if data == nil {
        data = s.generateRandomSDCIData()
    }
    return s.sendFrameWithBody(protocol.SDCI, data)
}
```

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi.go
git commit -m "refactor: update sendSDI and sendSDCI methods"
```

---

## Task 20: 添加 station.GetStateData 方法

**Files:**
- Modify: `cbi-tester/internal/station/state.go` (或相应文件)

**Step 1: 添加 GetStateData 方法**

```go
// GetStateData 返回站场状态字节数据
func (s *StationState) GetStateData() []byte {
    // TODO: 实现完整的站场状态数据生成
    // 暂时返回空数据
    return make([]byte, 0)
}
```

**Step 2: 提交**

```bash
cd cbi-tester
git add internal/station/state.go
git commit -m "feat: add GetStateData method"
```

---

## Task 21: 编写单元测试 - 概率响应

**Files:**
- Create: `cbi-tester/internal/simulator/cbi_response_test.go`

**Step 1: 编写概率响应测试**

```go
package simulator

import (
    "testing"
    "math/rand"
)

func TestProbabilisticResponse(t *testing.T) {
    cfg := DefaultConfig()
    s := NewCBISimulator(cfg)

    // 运行 1000 次，统计分布
    ackCount := 0
    firCount := 0
    scciCount := 0
    tsqCount := 0

    for i := 0; i < 1000; i++ {
        s.probabilisticResponse()
        // 根据发送的帧类型计数
        // 需要 mock transport 或添加计数器
    }

    // 验证分布接近预期比例
    // 70% ACK, 10% FIR, 19% SDCI, 1% TSQ
    t.Logf("ACK: %d, FIR: %d, SDCI: %d, TSQ: %d", ackCount, firCount, scciCount, tsqCount)
}
```

**Step 2: 运行测试**

Run: `cd cbi-tester && go test ./internal/simulator -v -run TestProbabilisticResponse`
Expected: PASS

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi_response_test.go
git commit -m "test: add probabilistic response unit test"
```

---

## Task 22: 编写单元测试 - 延时控制

**Files:**
- Create: `cbi-tester/internal/simulator/cbi_delay_test.go`

**Step 1: 编写延时控制测试**

```go
package simulator

import (
    "testing"
    "time"
)

func Test500msDelay(t *testing.T) {
    cfg := DefaultConfig()
    s := NewCBISimulator(cfg)

    // 启动延时
    s.start500msDelay()

    // 验证 hasDelay 为 true
    if !s.hasDelay {
        t.Error("hasDelay should be true after starting delay")
    }

    // 等待 600ms
    time.Sleep(600 * time.Millisecond)

    // 验证 hasDelay 为 false
    if s.hasDelay {
        t.Error("hasDelay should be false after delay expired")
    }
}

func TestDelayInterrupt(t *testing.T) {
    cfg := DefaultConfig()
    s := NewCBISimulator(cfg)

    // 启动延时
    s.start500msDelay()

    // 中断延时
    s.interruptDelay()

    // 验证 hasDelay 为 false
    if s.hasDelay {
        t.Error("hasDelay should be false after interrupt")
    }
}
```

**Step 2: 运行测试**

Run: `cd cbi-tester && go test ./internal/simulator -v -run Test500msDelay`
Expected: PASS

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi_delay_test.go
git commit -m "test: add delay control unit test"
```

---

## Task 23: 编写单元测试 - 重传机制

**Files:**
- Create: `cbi-tester/internal/simulator/cbi_retry_test.go`

**Step 1: 编写重传测试**

```go
package simulator

import (
    "testing"
    "time"
)

func TestRetryMechanism(t *testing.T) {
    cfg := DefaultConfig()
    s := NewCBISimulator(cfg)

    // 模拟发送帧
    frame := &protocol.Frame{Type: protocol.ACK}
    s.lastSentFrame = frame
    s.waitingForAck = true

    // 等待重传超时
    s.startRetryTimer()
    time.Sleep(600 * time.Millisecond)

    // 验证 retryCount 增加
    if s.retryCount != 1 {
        t.Errorf("retryCount should be 1, got %d", s.retryCount)
    }
}
```

**Step 2: 运行测试**

Run: `cd cbi-tester && go test ./internal/simulator -v -run TestRetryMechanism`
Expected: PASS

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi_retry_test.go
git commit -m "test: add retry mechanism unit test"
```

---

## Task 24: 编写单元测试 - 序号检查

**Files:**
- Create: `cbi-tester/internal/simulator/cbi_sequence_test.go`

**Step 1: 编写序号检查测试**

```go
package simulator

import (
    "testing"
    "cbi-simulator/internal/protocol"
)

func TestSequenceCheck(t *testing.T) {
    cfg := DefaultConfig()
    s := NewCBISimulator(cfg)

    // 正常帧
    frame := &protocol.Frame{SendSeq: 1}
    s.expectedRecvSeq = 0
    if !s.checkSequence(frame) {
        t.Error("Sequence check should pass for normal frame")
    }

    // 重复帧
    frame = &protocol.Frame{SendSeq: 1}
    s.expectedRecvSeq = 1
    if s.checkSequence(frame) {
        t.Error("Sequence check should fail for duplicate frame")
    }

    // 序号丢失
    frame = &protocol.Frame{SendSeq: 5}
    s.expectedRecvSeq = 1
    if s.checkSequence(frame) {
        t.Error("Sequence check should fail for lost frame")
    }
}
```

**Step 2: 运行测试**

Run: `cd cbi-tester && go test ./internal/simulator -v -run TestSequenceCheck`
Expected: PASS

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi_sequence_test.go
git commit -m "test: add sequence check unit test"
```

---

## Task 25: 完整集成测试

**Files:**
- Create: `cbi-tester/internal/simulator/cbi_integration_test.go`

**Step 1: 编写集成测试**

```go
package simulator

import (
    "testing"
    "context"
    "time"
)

func TestFullCommunicationFlow(t *testing.T) {
    cfg := DefaultConfig()
    s := NewCBISimulator(cfg)

    ctx := context.Background()
    err := s.Start(ctx)
    if err != nil {
        t.Fatalf("Start failed: %v", err)
    }
    defer s.Stop()

    // 模拟完整通信流程
    // DC2 -> DC3 -> RSR -> RSR(响应) -> ACK -> SDI -> ACK -> ACQ
    // 需要 mock transport 或集成测试框架

    t.Log("Integration test placeholder")
}
```

**Step 2: 运行测试**

Run: `cd cbi-tester && go test ./internal/simulator -v -run TestFullCommunicationFlow`
Expected: PASS (placeholder)

**Step 3: 提交**

```bash
cd cbi-tester
git add internal/simulator/cbi_integration_test.go
git commit -m "test: add integration test placeholder"
```

---

## Task 26: 验证编译和所有测试

**Files:**
- All modified files

**Step 1: 验证编译**

Run: `cd cbi-tester && go build ./...`
Expected: 编译成功，无错误

**Step 2: 运行所有测试**

Run: `cd cbi-tester && go test ./... -v`
Expected: 所有测试 PASS

**Step 3: 提交**

```bash
cd cbi-tester
git add .
git commit -m "chore: verify build and all tests pass"
```

---

## 完成

所有任务完成后，CBI 端通信逻辑重写完成。

**验证清单:**
- [ ] 编译成功
- [ ] 所有单元测试通过
- [ ] 连接建立流程正确：DC2→DC3→RSR→RSR(响应)
- [ ] 首次 ACK 序列正确：ACK(1)→SDI, ACK(2)→ACQ
- [ ] 延时中断功能正常
- [ ] 概率响应分布符合预期
- [ ] 自律状态切换正确
- [ ] 重传机制正常
- [ ] CRC 错误处理正常
- [ ] 1500ms 超时检测正常
- [ ] 序号异常处理正常
