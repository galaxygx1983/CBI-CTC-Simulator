# CBI 异常通信场景模拟 - 设计方案

| 版本 | 日期       | 作者 | 说明 |
|------|------------|------|------|
| v1.0 | 2026-03-20 |      | 初稿 |

---

## 1. 背景与目的

`cbi-client` 是 CBI（计算机联锁）模拟器客户端，用于与 CTC（调度集中）系统进行帧协议通信测试。当前客户端仅支持正常通信流程，测试人员无法便捷地模拟各类异常通信场景（如网络延迟、丢包、序号错乱、VERROR 断开等）。

本方案旨在通过启动参数（`--fault-*`）的方式，向 `cbi-client connect` 命令注入各类通信异常，无需修改核心业务代码，即可覆盖以下测试需求：

- ACK 超时与重传
- NACK 连续触发导致断开
- 序号错乱导致通信中断
- VERROR 版本错误
- 主动断连与重连
- 帧数据损坏与异常
- 网络延迟与丢包

---

## 2. 设计原则

1. **非侵入性** - 核心帧处理逻辑（`frame_handler.go`、`connect.go` 回调）保持不变，异常注入代码独立封装。
2. **参数化** - 所有异常行为均可通过命令行参数开启/关闭，支持组合使用。
3. **向后兼容** - 不传递任何 `--fault-*` 参数时，行为与原版完全一致。
4. **可调试** - 每个注入的异常在日志中明确标记，便于定位问题。
5. **可扩展** - 新增异常场景只需实现 `FaultInjector` 接口，无需修改已有代码。

---

## 3. 架构设计

### 3.1 模块结构

```
cmd/cbi-client/
├── connect.go              # [修改] 新增 faultConfig 解析和注入逻辑调用点
├── fault/
│   ├── injector.go        # [新增] FaultInjector 接口与实现
│   ├── config.go          # [新增] FaultConfig 配置结构体与 Flag 定义
│   └── scenarios.go       # [新增] 预定义场景组合
```

### 3.2 异常注入点

```
CTC ──────────▶ FrameHandler.HandleFrame()
                     │
                     ├── [注入点 A] 收帧前：丢帧、随机拒绝
                     │
                     ├── [注入点 B] 序号检查后：序号篡改、数据篡改
                     │
                     └── [注入点 C] 回调触发前：阻断 DC3/ACK/RSR
                                         ▼
              ┌────────────────────────┴────────────────────────┐
              │          回调区 (runConnect/onXXX)               │
              │  onDC2 / onRSR / onACK / onACA / onTSD / ...   │
              │  [注入点 D] 回复前：延时、替换帧类型、空数据    │
              └────────────────────────────────────────────────┘
                                         ▼
              ┌────────────────────────────────────────────────┐
              │         FrameHandler.SendDataFrame()           │
              │  [注入点 E] 发送前：版本号篡改、序号篡改       │
              └────────────────────────────────────────────────┘
```

### 3.3 注入点详解

| 注入点 | 位置 | 作用 |
|--------|------|------|
| A | `FrameHandler.HandleFrame()` 入口 | 随机丢帧（`random-drop`）、随机 NACK（`nack-random`） |
| B | `FrameHandler.checkDataFrameSequence()` 后 | 序号跳跃（`seq-skip`）、序号卡死（`seq-stuck`）、AckSeq 偏移（`seq-mismatch`） |
| C | `handleDC2()` 回调前 | 阻断 DC3 回复（`block-dc2`）、回复 VERROR（`verror`） |
| D | `runConnect` 回调函数中 | 延时（`delay`）、空数据（`empty-data`）、替换帧类型（`wrong-type`）、NACK 回复（`nack-after`） |
| E | `FrameHandler.SendDataFrame()` | 错误版本号（`wrong-version`） |

### 3.4 数据流

```
                 FaultConfig（启动参数）
                        │
                        ▼
              ┌─────────────────┐
              │  parseFaultFlags│  (connect.go init)
              └────────┬────────┘
                       │ *FaultInjector
          ┌────────────┴────────────┐
          │                           │
          ▼                           ▼
  ┌───────────────┐         ┌─────────────────┐
  │FrameHandler   │         │  runConnect     │
  │.HandleFrame() │         │  回调函数        │
  │  [注入点 A,B,E]│         │  [注入点 C,D]   │
  └───────────────┘         └─────────────────┘
```

---

## 4. FaultConfig 配置结构

### 4.1 配置字段

```go
// cmd/cbi-client/fault/config.go
type FaultConfig struct {
    // === 时序类 ===
    AckTimeout      int  // ACK 超时时间（ms），默认 490
    ReplyDelay      int  // 所有回复统一延时（ms），0=使用默认值 10
    ReplyDrop       bool // 收到帧后不回复
    RandomDrop      int  // 丢帧概率 0-100（%）

    // === 序号类 ===
    SeqSkip         int  // 每发送 N 帧跳过 1 个序号，0=不跳过
    SeqStuck        bool // 序号卡住不递增
    SeqMismatch     bool // AckSeq 故意不匹配

    // === 帧类型类 ===
    NackAfter       int  // 收到 N 帧后开始回复 NACK
    NackRandom      int  // N% 概率将正常回复替换为 NACK
    Verror          bool // 收到 DC2 后回复 VERROR
    BlockDC2        bool // 收到 DC2 后不回复 DC3
    WrongVersion    bool // 发送帧使用错误版本号 0x10

    // === 数据类 ===
    CorruptData     bool // SDI/SDCI 数据随机损坏
    EmptyData       bool // 所有发送帧数据长度为 0
    ExtraData       bool // 数据长度字段与实际不符

    // === 连接类 ===
    DisconnectAfter int  // N 秒后主动断开，0=不断开
    ReconnectLoop   bool // 断开后自动重连循环
}
```

### 4.2 命令行参数

所有参数以 `--fault-` 为前缀，支持组合使用。

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--fault ack-timeout` | int | 490 | ACK 超时毫秒数 |
| `--fault delay` | int | 0 | 统一延时（ms），0=默认10ms |
| `--fault reply-drop` | flag | false | 收到帧后不回复任何帧 |
| `--fault random-drop` | int | 0 | 丢帧概率 0-100% |
| `--fault seq-skip` | int | 0 | 每N帧跳过1序号，0=不跳过 |
| `--fault seq-stuck` | flag | false | 序号卡在当前值不递增 |
| `--fault seq-mismatch` | flag | false | AckSeq 故意偏移 |
| `--fault nack-after` | int | 0 | 收到N帧后开始回复NACK，0=不启用 |
| `--fault nack-random` | int | 0 | N%概率替换为NACK |
| `--fault verror` | flag | false | 收到DC2后回复VERROR |
| `--fault block-dc2` | flag | false | 收到DC2后不回复DC3 |
| `--fault wrong-version` | flag | false | 发送帧使用错误版本号 |
| `--fault corrupt-data` | flag | false | 数据内容随机损坏 |
| `--fault empty-data` | flag | false | 所有帧数据长度为0 |
| `--fault extra-data` | flag | false | 数据长度字段与实际不符 |
| `--fault disconnect-after` | int | 0 | N秒后断开，0=不断开 |
| `--fault reconnect-loop` | flag | false | 断开后自动重连 |

---

## 5. FaultInjector 接口设计

### 5.1 接口定义

```go
// cmd/cbi-client/fault/injector.go
type FaultInjector struct {
    cfg     *FaultConfig
    stats   *FaultStats        // 统计计数（便于日志输出）
    seqSent int                // 已发送帧计数（用于 seq-skip / nack-after）
    seqMu   sync.Mutex
}

type FaultStats struct {
    DroppedFrames     int64 // 丢弃的帧数
    DelayedFrames     int64 // 延时发送的帧数
    CorruptedFrames   int64 // 损坏的帧数
    NackSent          int64 // NACK 发送次数
    SeqSkipped        int64 // 序号跳过次数
    DisconnectTrigger int64 // 主动断连次数
}

type InjectionResult struct {
    Block  bool            // true = 拦截该帧，不发送/不处理
    Nack   bool            // true = 替换为 NACK
    Verror bool            // true = 替换为 VERROR
    Delay  time.Duration   // 额外的延时
    Corrupt bool           // true = 篡改数据内容
}
```

### 5.2 核心方法

```go
// BeforeSend 发送帧前调用 - 处理序号篡改、版本篡改、数据篡改
// 注入点 E
func (fi *FaultInjector) BeforeSend(frame *protocol.Frame) {
    // 1. WrongVersion: 篡改版本号
    if fi.cfg.WrongVersion {
        frame.Version = 0x10  // 错误版本
    }
    // 2. SeqStuck: 序号卡住（不递增已在发送后执行，这里标记是否跳过本次递增）
    // 3. ExtraData: 篡改数据长度字段（不影响 Data 实际内容）
    if fi.cfg.ExtraData && len(frame.Data) > 0 {
        frame.DataLength = uint16(len(frame.Data) + 10)  // 长度比实际大
    }
}

// AfterSend 发送帧后调用 - 序号跳跃控制
func (fi *FaultInjector) AfterSend(frame *protocol.Frame) {
    fi.seqMu.Lock()
    fi.seqSent++
    // SeqSkip: 每 N 帧跳过 1 个序号
    if fi.cfg.SeqSkip > 0 && fi.seqSent % fi.cfg.SeqSkip == 0 {
        fi.skipNextSeq = true  // 标记下次跳过
        fi.stats.SeqSkipped++
    }
    fi.seqMu.Unlock()
}

// BeforeRecv 接收帧后、序号检查前调用 - 丢帧、NACK 随机替换
// 注入点 A
func (fi *FaultInjector) BeforeRecv(frame *protocol.Frame) InjectionResult {
    result := InjectionResult{}
    fi.seqMu.Lock()
    defer fi.seqMu.Unlock()

    // RandomDrop: 随机丢帧
    if fi.cfg.RandomDrop > 0 && rand.Intn(100) < fi.cfg.RandomDrop {
        fi.stats.DroppedFrames++
        result.Block = true
        return result
    }

    // NackRandom: 随机 NACK
    if fi.cfg.NackRandom > 0 && rand.Intn(100) < fi.cfg.NackRandom {
        result.Nack = true
        fi.stats.NackSent++
        return result
    }

    return result
}

// AfterRecv 接收帧后、序号检查后调用 - 序号错乱后的数据篡改
// 注入点 B
func (fi *FaultInjector) AfterRecvCheck(frame *protocol.Frame) {
    // CorruptData: 随机损坏数据内容
    if fi.cfg.CorruptData && len(frame.Data) > 0 {
        pos := rand.Intn(len(frame.Data))
        frame.Data[pos] ^= 0xFF  // 位翻转
        fi.stats.CorruptedFrames++
    }
}

// ShouldReplyDrop 判断是否阻断回复
func (fi *FaultInjector) ShouldReplyDrop() bool {
    return fi.cfg.ReplyDrop
}

// ShouldBlockDC2 判断是否阻断 DC3 回复
func (fi *FaultInjector) ShouldBlockDC2() bool {
    return fi.cfg.BlockDC2
}

// ShouldSendVerrorOnDC2 判断收到 DC2 是否回复 VERROR
func (fi *FaultInjector) ShouldSendVerrorOnDC2() bool {
    return fi.cfg.Verror
}

// GetReplyDelay 获取回复延时
func (fi *FaultInjector) GetReplyDelay() time.Duration {
    if fi.cfg.ReplyDelay > 0 {
        return time.Duration(fi.cfg.ReplyDelay) * time.Millisecond
    }
    return defaultDelay  // 10ms
}

// GetAckTimeout 获取 ACK 超时时间
func (fi *FaultInjector) GetAckTimeout() time.Duration {
    if fi.cfg.AckTimeout > 0 {
        return time.Duration(fi.cfg.AckTimeout) * time.Millisecond
    }
    return 490 * time.Millisecond
}

// ShouldSendNackAfter 判断当前是否应发送 NACK
func (fi *FaultInjector) ShouldSendNackAfter() bool {
    if fi.cfg.NackAfter == 0 {
        return false
    }
    fi.seqMu.Lock()
    defer fi.seqMu.Unlock()
    return fi.seqSent >= fi.cfg.NackAfter
}

// ShouldEmptyData 判断是否返回空数据
func (fi *FaultInjector) ShouldEmptyData() bool {
    return fi.cfg.EmptyData
}

// ShouldDisconnect 判断是否应主动断开
func (fi *FaultInjector) ShouldDisconnect() bool {
    if fi.cfg.DisconnectAfter == 0 {
        return false
    }
    // 定时器由调用方在 runConnect 中启动
    return true
}
```

---

## 6. 修改明细

### 6.1 新增文件

#### `cmd/cbi-client/fault/config.go`

定义 `FaultConfig` 结构体、`ParseFlags()` 函数、`RegisterFlags()` 方法。

#### `cmd/cbi-client/fault/injector.go`

定义 `FaultInjector` 结构体及其所有方法。

#### `cmd/cbi-client/fault/scenarios.go`

预定义场景组合函数：

```go
func NetworkCongestion() *FaultConfig   // delay=2000, random-drop=10
func AckTimeout() *FaultConfig           // ack-timeout=2000
func NackAttack() *FaultConfig           // nack-after=2
func SeqDisorder() *FaultConfig          // seq-skip=3
func VersionMismatch() *FaultConfig      // wrong-version
func DataCorruption() *FaultConfig      // corrupt-data
func DisconnectLoop() *FaultConfig      // disconnect-after=5, reconnect-loop
func VerrorDisconnect() *FaultConfig     // verror
```

---

### 6.2 修改文件

#### `cmd/cbi-client/connect.go`

**变更 1** - 新增导入和变量：

```go
import (
    "cbi-simulator/fault"
)

var (
    // 已有字段...
    faultConfig = fault.NewFaultConfig()
)
```

**变更 2** - 新增 Flag 注册（`init()` 函数中）：

```go
func init() {
    // 已有代码...
    faultConfig.RegisterFlags(connectCmd)
}
```

**变更 3** - `runConnect` 函数中，创建 Injector 并注入：

```go
// 创建异常注入器
injector := fault.NewFaultInjector(faultConfig)

// 通知 FrameHandler 使用新的 ACK 超时时间
handler.SetAckTimeout(injector.GetAckTimeout())

// onDC2 回调中注入
handler.OnDC2(func(frame *protocol.Frame) {
    if injector.ShouldBlockDC2() {
        log.Warn("[FAULT] DC2 blocked")
        return
    }
    if injector.ShouldSendVerrorOnDC2() {
        log.Warn("[FAULT] Sending VERROR on DC2")
        handler.SendVERROR()
        handler.Disconnect()
        return
    }
    // 正常流程...
    time.Sleep(injector.GetReplyDelay())
    handler.SendDC3()
})

// onACK 回调中注入
handler.OnACK(func(frame *protocol.Frame) {
    if injector.ShouldReplyDrop() {
        log.Warn("[FAULT] Reply dropped")
        return
    }
    if injector.ShouldSendNackAfter() {
        log.Warn("[FAULT] Injecting NACK")
        injector.RecordNack()
        handler.SendNACK()
        return
    }
    if injector.ShouldEmptyData() {
        // 回调返回空数据
    }
    // ... 正常流程
})

// 发送帧前注入
client.SetOnFrameSent(func(frame *protocol.Frame) {
    injector.BeforeSend(frame)
    injector.AfterSend(frame)
})

// 接收帧后注入（丢帧/NACK随机）
client.SetOnFrameReceived(func(frame *protocol.Frame) {
    result := injector.BeforeRecv(frame)
    if result.Block {
        log.Warnf("[FAULT] Frame %s dropped", frame.Type)
        return
    }
    if result.Nack {
        log.Warnf("[FAULT] Frame %s replaced with NACK", frame.Type)
        handler.SendNACK()
        return
    }
    // 正常流程...
    frameLogger.LogFrameRecv(byte(frame.Type), frameData)
})

// 主动断连定时器
if injector.ShouldDisconnect() {
    time.AfterFunc(time.Duration(faultConfig.DisconnectAfter)*time.Second, func() {
        log.Warn("[FAULT] Triggering disconnect")
        injector.RecordDisconnect()
        client.Disconnect()
        if faultConfig.ReconnectLoop {
            // 重连循环逻辑
        }
    })
}
```

#### `cmd/cbi-client/fault/config.go`（新增）

```go
func (fc *FaultConfig) RegisterFlags(cmd *cobra.Command) {
    cmd.Flags().IntVar(&fc.AckTimeout, "fault-ack-timeout", 490, "ACK timeout in ms")
    cmd.Flags().IntVar(&fc.ReplyDelay, "fault-delay", 0, "Reply delay in ms (0=default 10ms)")
    cmd.Flags().BoolVar(&fc.ReplyDrop, "fault-reply-drop", false, "Drop all replies")
    cmd.Flags().IntVar(&fc.RandomDrop, "fault-random-drop", 0, "Random drop probability 0-100%")
    cmd.Flags().IntVar(&fc.SeqSkip, "fault-seq-skip", 0, "Skip seq every N frames")
    cmd.Flags().BoolVar(&fc.SeqStuck, "fault-seq-stuck", false, "Fix seq at current value")
    cmd.Flags().BoolVar(&fc.SeqMismatch, "fault-seq-mismatch", false, "Offset ackSeq")
    cmd.Flags().IntVar(&fc.NackAfter, "fault-nack-after", 0, "Send NACK after N frames")
    cmd.Flags().IntVar(&fc.NackRandom, "fault-nack-random", 0, "NACK probability 0-100%")
    cmd.Flags().BoolVar(&fc.Verror, "fault-verror", false, "Reply VERROR on DC2")
    cmd.Flags().BoolVar(&fc.BlockDC2, "fault-block-dc2", false, "Block DC3 reply on DC2")
    cmd.Flags().BoolVar(&fc.WrongVersion, "fault-wrong-version", false, "Use wrong version 0x10")
    cmd.Flags().BoolVar(&fc.CorruptData, "fault-corrupt-data", false, "Corrupt SDI/SDCI data")
    cmd.Flags().BoolVar(&fc.EmptyData, "fault-empty-data", false, "Send frames with empty data")
    cmd.Flags().BoolVar(&fc.ExtraData, "fault-extra-data", false, "Set data length mismatch")
    cmd.Flags().IntVar(&fc.DisconnectAfter, "fault-disconnect-after", 0, "Disconnect after N seconds")
    cmd.Flags().BoolVar(&fc.ReconnectLoop, "fault-reconnect-loop", false, "Auto reconnect after disconnect")
}
```

---

### 6.3 FrameHandler 改造

`frame_handler.go` 需新增一个 `SetAckTimeout` 方法，以便外部注入自定义超时：

```go
// SetAckTimeout 设置 ACK 超时时间（用于故障注入）
func (h *FrameHandler) SetAckTimeout(timeout time.Duration) {
    h.nextFrameMu.Lock()
    h.customAckTimeout = timeout
    h.nextFrameMu.Unlock()
}

// updateNextFrameSendTime 修改为支持自定义超时
func (h *FrameHandler) updateNextFrameSendTime() {
    h.nextFrameMu.Lock()
    defer h.nextFrameMu.Unlock()
    timeout := 490 * time.Millisecond
    if h.customAckTimeout > 0 {
        timeout = h.customAckTimeout
    }
    h.nextFrameSendTime = time.Now().Add(timeout)
}
```

---

## 7. 使用示例

```bash
# 正常模式（向后兼容）
cbi-client connect -a localhost:50051

# 场景1：网络延迟（ACK超时设置为2秒）
cbi-client connect -a localhost:50051 \
    --fault-ack-timeout 2000

# 场景2：网络拥塞（延迟2秒 + 10%丢包）
cbi-client connect -a localhost:50051 \
    --fault-delay 2000 \
    --fault-random-drop 10

# 场景3：NACK 攻击（收到2帧后开始发NACK）
cbi-client connect -a localhost:50051 \
    --fault-nack-after 2

# 场景4：序号错乱（每3帧跳过1个序号）
cbi-client connect -a localhost:50051 \
    --fault-seq-skip 3

# 场景5：版本错误（导致CTC发送VERROR断开）
cbi-client connect -a localhost:50051 \
    --fault-wrong-version

# 场景6：VERROR 攻击（CBI主动发VERROR断开）
cbi-client connect -a localhost:50051 \
    --fault-verror

# 场景7：反复断连重连
cbi-client connect -a localhost:50051 \
    --fault-disconnect-after 5 \
    --fault-reconnect-loop

# 场景8：数据损坏（SDI/SDCI数据内容异常）
cbi-client connect -a localhost:50051 \
    --fault-corrupt-data

# 场景9：复合场景（延迟 + NACK + 丢包）
cbi-client connect -a localhost:50051 \
    --fault-delay 1000 \
    --fault-nack-random 20 \
    --fault-random-drop 5

# 场景10：使用预定义场景
cbi-client connect -a localhost:50051 \
    --fault-scenario network-congestion
```

---

## 8. 兼容性说明

- 不传递任何 `--fault-*` 参数时，所有字段为零值，`FaultInjector` 所有判断均返回 `false`，行为与原版完全一致。
- `faultConfig` 字段的零值对应正常行为（`AckTimeout=0` → 使用默认 490ms，`Delay=0` → 使用默认 10ms），无需特殊处理。

---

## 9. 风险与限制

1. **序号卡死（SeqStuck）** 与 **序号跳跃（SeqSkip）** 不能同时启用。
2. **ReplyDrop** 与 **NackAfter** 同时启用时，NACK 也会被丢弃，导致 CTC 侧 ACK 超时而非 NACK 处理。
3. **WrongVersion** 和 **CorruptData** 会导致对方 VERROR 断开，并非本端主动断开，重连后仍会复现。
4. `frame_handler.go` 中 `customAckTimeout` 字段需确认线程安全（已有 `nextFrameMu` 保护）。

---

## 10. 测试计划

| 测试项 | 方法 |
|--------|------|
| 参数解析 | 逐一传递每个 `--fault-*` 参数，验证解析正确 |
| ACK 超时 | `--fault-ack-timeout 100` 观察 CTC 侧超时日志 |
| 丢帧 | `--fault-random-drop 100` 观察 CTC 持续超时 |
| 序号跳跃 | `--fault-seq-skip 1` 观察通信中断日志 |
| VERROR | `--fault-verror` 观察 CTC 主动断开并输出 VERROR |
| 断连重连 | `--fault-disconnect-after 3 --fault-reconnect-loop` 观察反复连接 |
| 数据损坏 | `--fault-corrupt-data` 用日志分析工具验证 CRC 正确但内容异常 |
| 复合场景 | 组合 3+ 个参数，验证互不干扰 |
| 向后兼容 | 不带任何 `--fault-*` 参数，验证正常通信 |

---

## 11. 附录：帧类型参考

| 类型 | 代码 | 方向 | 说明 |
|------|------|------|------|
| DC2 | 0x12 | CTC→CBI | 连接请求 |
| DC3 | 0x13 | CBI→CTC | 连接确认 |
| ACK | 0x06 | 双向 | 应答/心跳 |
| NACK | 0x15 | 双向 | 否定应答 |
| VERROR | 0x10 | 双向 | 版本错误 |
| SDCI | 0x8A | CBI→CTC | 站场数据变化（增量） |
| SDI | 0x85 | CBI→CTC | 站场完整数据（全量） |
| SDIQ | 0x6A | CTC→CBI | 站场数据请求 |
| FIR | 0x65 | CBI→CTC | 故障信息报告 |
| RSR | 0xAA | 双向 | 系统工作状态报告 |
| BCC | 0x95 | CTC→CBI | 按钮控制命令 |
| ACQ | 0x75 | CBI→CTC | 自律控制请求 |
| ACA | 0x7A | CTC→CBI | 自律控制同意 |
| TSQ | 0x9A | CBI→CTC | 时间同步请求 |
| TSD | 0xA5 | CTC→CBI | 时间同步数据 |
