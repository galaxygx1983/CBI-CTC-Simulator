# CBI-CTC 通信逻辑重设计（CBI 端）

**日期**: 2026-03-18
**作者**: Claude Code
**状态**: 已批准
**目标**: 重写 `cbi-tester` 中的 CBI 通信逻辑

---

## 一、概述

本设计文档描述了 CBI-CTC Simulator 中 **CBI 端**（联锁端）通信逻辑的完全重写方案。CBI 作为服务器/被控端，响应 CTC 发送的各种帧。

---

## 二、目标文件

**修改文件**:
- `cbi-tester/internal/simulator/cbi.go` - CBI 模拟器核心逻辑
- `cbi-tester/internal/grpc/frame_handler.go` - 帧处理器（可选）

---

## 三、状态机设计

### 3.1 CBISimulator 新增字段

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
    sendSeq   byte
    ackSeq    byte
    seqMu     sync.Mutex
    onFrameReceived func(frame *protocol.Frame)

    // 通信状态（新增）
    firstRSRReceived   bool           // 是否已收到第一个 RSR
    ackCount           int            // 收到 ACK 的次数（收到 DC2 时重置）
    autoControlAgreed  bool           // ACA 是否已同意自律控制

    // 延时控制（新增）
    hasDelay   bool                    // 是否有 500ms 延时正在运行
    delayTimer *time.Timer             // 500ms 延时计时器

    // 异常计数（新增）
    crcErrorCount      int             // 连续 CRC 错误计数
    lastRecvTime       time.Time       // 最后接收帧时间（1500ms 超时检测）

    // 重传状态（新增）
    lastSentFrame      *protocol.Frame // 最近发送的帧（用于重传）
    retryCount         int             // 重传次数（0-2）
    retryTimer         *time.Timer     // 重传计时器
    waitingForAck      bool            // 是否等待 ACK

    // 序号跟踪（新增）
    expectedRecvSeq    byte            // 期望接收的序号
}
```

---

## 四、帧处理逻辑（CBI 端）

### 4.1 帧类型响应真值表

| 接收帧 | 条件 | 动作 | 响应 |
|--------|------|------|------|
| **DC2** | 任意 | 重置所有状态 | 立即回复 DC3 |
| **RSR** | 首次 (!firstRSRReceived) | firstRSRReceived=true | 立即回复 RSR(主机/非常站控=0x55,0xAA) |
| **RSR** | 后续 | 中断延时 | 概率响应 (70% ACK / 10% FIR / 19% SDCI / 1% TSQ) |
| **ACK** | 第 1 次 (ackCount==0) | ackCount=1 | 立即回复 SDI |
| **ACK** | 第 2 次 (ackCount==1) | ackCount=2 | 立即回复 ACQ |
| **ACK** | 后续 (ackCount>=2) | ackCount++ | 无延时：70% 启动延时 / 10% FIR / 15% SDCI / 5% TSQ<br>有延时：不处理 |
| **NACK** | 任意 | retryCount++ | 重发 lastSentFrame（序号/数据不变，更新 ackSeq） |
| **VERROR** | 任意 | - | 判定通信中断，触发 DC2 重连 |
| **SDIQ** | 任意 | 中断延时 | 立即回复 SDI |
| **BCC** | 任意 | 中断延时 | 概率响应 (70% ACK / 10% FIR / 19% SDCI / 1% TSQ) |
| **TSD** | 任意 | 中断延时 | 概率响应 (70% ACK / 10% FIR / 20% SDCI) |
| **ACA** | 任意 | 中断延时 + autoControlAgreed=true | 概率响应 (70% ACK / 10% FIR / 19% SDCI / 1% TSQ) |
| **SDCI** | 不可能收到 | CBI 是发送方，不处理接收 | - |
| **FIR** | 不可能收到 | CBI 是发送方，不处理接收 | - |
| **TSQ** | 不可能收到 | CBI 是发送方，不处理接收 | - |
| **ACQ** | 不可能收到 | CBI 是发送方，不处理接收 | - |
| **TSD** | 任意 | 中断延时 | 概率响应 (70% ACK / 10% FIR / 20% SDCI) |

**CBI 端可能收到的帧**: DC2、RSR、ACK、NACK、VERROR、SDIQ、BCC、TSD、ACA

**CBI 端发送的帧** (不会收到): DC3、RSR、SDI、SDCI、FIR、TSQ、TSD、ACQ、ACA

### 4.2 异常处理

| 异常类型 | 动作 | 响应 | 阈值 |
|----------|------|------|------|
| CRC 校验错误 | crcErrorCount++ | 立即回复 NACK | >=5 → 中断通信 |
| 版本号错误 | - | 立即回复 VERROR | - |
| 序号不正确 | 序号异常处理 | 重复帧：回复 ACK<br>序号丢失：DC2 重连 | - |
| 1500ms 超时 | - | 中断通信（DC2 重连） | - |

---

## 五、概率响应实现

### 5.1 通用概率响应

适用于：BCC、TSD、RSR（后续）、ACA

```go
func (s *CBISimulator) probabilisticResponse() {
    randVal := rand.Intn(100) // 0-99

    switch {
    case randVal < 70:
        s.sendACK()          // 0-69: 70%
    case randVal < 80:
        s.sendFIR()          // 70-79: 10%
    case randVal < 99:
        s.sendSDCI()         // 80-98: 19%
    default:
        s.sendTSQ()          // 99: 1%
    }
}
```

### 5.2 ACK 特殊概率响应

```go
func (s *CBISimulator) probabilisticResponseForACK() {
    randVal := rand.Intn(100)

    switch {
    case randVal < 70:
        s.start500msDelay()  // 0-69: 70%
    case randVal < 80:
        s.sendFIR()          // 70-79: 10%
    case randVal < 95:
        s.sendSDCI()         // 80-94: 15%
    default:
        s.sendTSQ()          // 95-99: 5%
    }
}
```

---

## 六、延时控制机制

### 6.1 启动 500ms 延时

```go
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
```

### 6.2 中断延时

```go
func (s *CBISimulator) interruptDelay() {
    if s.hasDelay && s.delayTimer != nil {
        s.delayTimer.Stop()
        s.hasDelay = false
    }
}
```

---

## 七、重传机制

### 7.1 发送方时序图

```
T=0ms     发送数据帧（发送序号=10）
T=500ms   超时！未收到 ACK
T=500ms   重发相同帧（第 2 次尝试）
T=1000ms  超时！仍未收到 ACK
T=1000ms  重发相同帧（第 3 次尝试）
T=1500ms  超时！3 次尝试均失败
T=1500ms  → 判定通信中断，发送 DC2 重连
```

### 7.2 重传字段要求

| 字段 | 是否允许修改 | 备注 |
|------|---------------|------|
| 发送序号 | ❌ 否 | 必须与原始帧相同 |
| 数据内容 | ❌ 否 | 必须完全相同 |
| 确认序号 | ✅ 是 | 更新为最近收到的帧 |
| CRC | ✅ 是 | 确认序号变化时需重新计算 |

---

## 八、NACK 处理

```go
func (s *CBISimulator) handleNACK(frame *protocol.Frame) {
    log.Info("Received NACK, resending last frame")

    if s.lastSentFrame == nil {
        log.Warn("No frame to resend")
        return
    }

    // 重发上一帧（序号/数据不变，更新 ackSeq）
    frame := s.lastSentFrame
    frame.AckSeq = s.ackSeq
    frame.CRC = 0

    s.sendFrame(frame)
    s.startRetryTimer()
}
```

---

## 九、CRC 校验错误处理

```go
func (s *CBISimulator) handleData(data []byte) {
    frame, err := protocol.DecodeFrame(data)
    if err != nil {
        // 校验 CRC
        if !s.verifyCRC(frame) {
            s.crcErrorCount++
            log.Warnf("CRC error (count=%d)", s.crcErrorCount)

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
    }

    // ... 继续处理帧 ...
}
```

---

## 十、版本号检查

```go
if frame.Version != protocol.Version {
    log.Warnf("Version mismatch: got 0x%02X, expect 0x%02X", frame.Version, protocol.Version)
    s.sendVERROR()
    return
}
```

---

## 十一、序号异常处理

```go
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

case seqDiff == 0:
    // 重复帧
    log.Warnf("Duplicate frame: %s (seq=%d)", frame.Type, frame.SendSeq)
    s.sendACK() // 回复 ACK 确认

default:
    // 序号跳变/丢失
    log.Errorf("Frame loss detected: expected seq=%d, got seq=%d", expectedSeq, frame.SendSeq)
    s.sendDC2Reconnect()
}
```

---

## 十二、1500ms 超时检测

```go
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

---

## 十三、数据生成

### 13.1 随机 SDCI 数据

```go
func (s *CBISimulator) generateRandomSDCIData() []byte {
    count := rand.Intn(10) + 1  // 1-10 个数据项
    data := make([]byte, count*3)
    for i := range data {
        data[i] = byte(rand.Intn(256))
    }
    return data
}
```

### 13.2 随机 FIR 数据

```go
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

---

## 十四、RSR 模式切换

### 14.1 首次 RSR 响应

```go
func (s *CBISimulator) sendRSRForInitialMode() {
    // 主机 + 非常站控模式
    data := []byte{0x55, 0xAA}
    s.sendFrameWithBody(protocol.RSR, data)
}
```

### 14.2 后续 RSR 响应

```go
func (s *CBISimulator) buildRSR() []byte {
    mode := byte(0xAA) // 默认非常站控
    if s.autoControlAgreed {
        mode = 0x55    // 自律控制模式
    }
    return []byte{0x55, mode} // 主机 + 模式
}
```

---

## 十五、Builder 扩展

需要在 `command/builder.go` 中添加：

```go
// BuildFIR 构建故障信息报告
func (b *Builder) BuildFIR(data []byte) *protocol.Frame {
    b.mu.Lock()
    defer b.mu.Unlock()

    frame := &protocol.Frame{
        Type:       protocol.FIR,
        SendSeq:    b.sendSeq,
        AckSeq:     b.ackSeq,
        DataLength: uint16(len(data)),
        Data:       data,
    }

    return frame
}

// BuildNACK 构建否定应答
func (b *Builder) BuildNACK() *protocol.Frame {
    b.mu.Lock()
    defer b.mu.Unlock()

    frame := &protocol.Frame{
        Type:    protocol.NACK,
        SendSeq: b.sendSeq,
        AckSeq:  b.ackSeq,
    }

    return frame
}

// BuildVERROR 构建版本错误
func (b *Builder) BuildVERROR() *protocol.Frame {
    b.mu.Lock()
    defer b.mu.Unlock()

    frame := &protocol.Frame{
        Type:    protocol.VERROR,
        SendSeq: b.sendSeq,
        AckSeq:  b.ackSeq,
    }

    return frame
}
```

---

## 十六、测试验证要点

1. **连接建立流程**：DC2→DC3→RSR→RSR(响应)
2. **首次 ACK 序列**：ACK(1)→SDI, ACK(2)→ACQ
3. **延时中断**：启动延时后收到 SDIQ/BCC/TSD，立即中断
4. **概率分布**：运行 1000 次统计各响应比例
5. **自律状态切换**：收到 ACA 后 RSR 模式变化
6. **重传机制**：500ms 超时重传，3 次失败后重连
7. **CRC 错误**：连续 5 次错误后重连
8. **1500ms 超时**：无数据接收时重连
9. **序号异常**：重复帧/序号丢失处理
10. **版本错误**：VERROR 响应

---

**设计批准**: 用户已确认以上设计符合要求。
