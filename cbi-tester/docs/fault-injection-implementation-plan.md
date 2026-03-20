# CBI 异常通信场景模拟 - 实现计划

| 版本 | 日期       | 作者 | 说明 |
|------|------------|------|------|
| v1.0 | 2026-03-20 |      | 初稿 |

---

## 概述

本文档将设计方案 `fault-injection-design.md` 转化为可分阶段执行的实现计划。

**前置条件**：
- Go 1.21+
- `cbi-client connect` 命令可正常编译运行
- `cbcd` 服务端可正常启动

**总工作量**：新增 3 个文件，修改 2 个已有文件，预计代码量约 600-800 行。

---

## 阶段划分

```
阶段一：基础设施（不依赖其他阶段）
  └── 1.1 新增 fault/config.go - FaultConfig 结构体、Flag 注册、hasAnyFaultFlag
  └── 1.2 新增 fault/injector.go - FaultInjector 核心骨架
  └── 1.3 单元测试：验证 Flag 解析正确性与 hasAnyFaultFlag 逻辑

阶段二：核心注入逻辑（依赖阶段一）
  └── 2.1 实现注入点 E：BeforeSend / AfterSend（版本号、序号跳跃）
  └── 2.2 实现注入点 A：BeforeRecv（丢帧、随机NACK）
  └── 2.3 实现注入点 B：AfterRecvCheck（数据损坏）
  └── 2.4 实现连接类注入：主动断连与重连循环
  └── 2.5 单元测试：验证注入逻辑

阶段三：与 connect.go 集成（依赖阶段二）
  └── 3.0 修改 connect.go - hasAnyFaultFlag 检测 + 条件化Injector创建（核心保证）
  └── 3.1 修改 connect.go - 在各回调中插入 nil-safe 注入逻辑
  └── 3.2 修改 frame_handler.go - 新增 SetAckTimeout（nil-safe）
  └── 3.3 新增 fault/scenarios.go - 预定义场景

阶段四：端到端测试（依赖阶段三）
  └── 4.0 正常模式保证验证：确认零参数时 injector 为 nil，所有注入点短路
  └── 4.1 正常模式回归测试
  └── 4.2 各异常参数单独验证
  └── 4.3 复合场景验证
```

---

## 正常场景保证机制（必须优先实现）

### 设计目标

**正常场景（不传递任何 `--fault-*` 参数）的代码路径与故障注入代码完全正交，两者在运行时无任何交集。**

具体表现为：
- 不传递任何 `--fault-*` 参数时，`FaultInjector` 实例永不创建
- `FrameHandler` 中的所有注入点均以 `if h.injector == nil { return }` 短路
- `connect.go` 中所有注入回调在 `injector == nil` 时完全不注册
- 最终效果：**零参数运行时的执行代码与原版完全相同（零差异）**

### 三层保证链

#### 保证一：Flag 存在性检测（入口拦截）

新增 `fault/config.go`：

```go
// hasAnyFaultFlag 检测是否有任何 --fault-* 参数被显式传递
// 注意：零值参数（如 --fault-ack-timeout 0）不算"显式传递"，因为 cobra 的 Changed 不会为默认值返回 true
func hasAnyFaultFlag(cmd *cobra.Command) bool {
    var found bool
    cmd.Flags().VisitAll(func(f *pflag.Flag) {
        if strings.HasPrefix(f.Name, "fault-") && f.Changed {
            found = true
        }
    })
    return found
}

// NewFaultInjectorOrNil 根据是否有 --fault-* 参数决定是否创建 Injector
// 无任何故障参数时返回 nil，保证零侵入
func NewFaultInjectorOrNil(cmd *cobra.Command) (*FaultInjector, error) {
    if !hasAnyFaultFlag(cmd) {
        return nil, nil // 关键：返回 nil 而非空 injector
    }
    cfg, err := NewFaultConfigFromCommand(cmd)
    if err != nil {
        return nil, err
    }
    return NewFaultInjector(cfg), nil
}
```

#### 保证二：Injector 为 nil 时的短路（所有注入点）

所有注入点（BeforeSend / BeforeRecv / ShouldBlockDC2 等）内部均有 nil guard：

```go
// 所有注入方法统一入口模式：
func (fi *FaultInjector) ShouldBlockDC2() bool {
    if fi == nil { // 关键保证
        return false
    }
    return fi.cfg.BlockDC2
}

func (fi *FaultInjector) BeforeSend(frame *protocol.Frame) {
    if fi == nil { // 关键保证
        return
    }
    // ... 实际注入逻辑
}
```

在 `connect.go` 中调用时无需判断 nil：

```go
// injector 为 nil 时，ShouldBlockDC2() 直接返回 false，行为与原版完全一致
if injector.ShouldBlockDC2() {
    // 这个分支永远不会执行，但编译器仍然允许这段代码存在
    // 零参数运行时，这里的条件永远为 false，无任何副作用
}
```

#### 保证三：条件化回调注册（connect.go）

```go
func runConnect(cmd *cobra.Command, args []string) {
    // 从 cmd 获取 injector（无任何 --fault-* 参数时为 nil）
    injector, err := fault.NewFaultInjectorOrNil(cmd)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create fault injector: %v\n", err)
        os.Exit(1)
    }

    // 创建日志记录器（与 injector 无关，始终执行）
    frameLogger, err := logger.NewLogger(...)
    // ...

    // 创建客户端和 handler（与 injector 无关，始终执行）
    client, err := grpc.NewClient(connectAddress)
    // ...
    handler := client.GetHandler()

    // 只有 injector != nil 时才注册注入回调
    if injector != nil {
        // 设置自定义 ACK 超时（nil-safe 内部实现）
        handler.SetAckTimeout(injector.GetAckTimeout())

        // 注册所有注入回调
        setupFaultInjections(client, handler, cbiState, injector, frameLogger)

        // 打印启用信息
        fmt.Printf("[FAULT INJECTION ENABLED] %s\n", injector.Config().String())
    }
    // ↑ 没有 injector 时：handler.SetAckTimeout 从不被调用，
    //   setupFaultInjections 从不被调用，connect.go 的行为与原版完全一致

    // 以下为所有回调注册（无条件执行，与 injector 无关）
    client.SetOnFrameReceived(func(frame *protocol.Frame) {
        // 注入点 A（nil-safe）
        if injector != nil {
            result := injector.BeforeRecv(frame)
            // ... 注入逻辑
        }
        // 原版日志记录逻辑（始终执行）
        frameData := protocol.FrameToBytes(frame)
        frameLogger.LogFrameRecv(byte(frame.Type), frameData)
        fmt.Printf("Received frame: %s\n", frame.Type)
    })

    // 正常连接流程...
}
```

### 完整的保证链条

```
正常场景：cbi-client connect -a localhost:50051
  └── hasAnyFaultFlag(cmd) → false
  └── NewFaultInjectorOrNil(cmd) → return nil, nil
  └── injector == nil
  └── handler.SetAckTimeout() → 从不被调用（FrameHandler 使用默认 490ms）
  └── setupFaultInjections() → 从不被调用（无任何注入回调注册）
  └── client.SetOnFrameReceived 中的 injector != nil 判断 → false，注入逻辑被跳过
  └── FrameHandler.SendDataFrame 中 h.injector != nil 判断 → false，BeforeSend 被跳过
  └── connect.go 实际执行路径与原版代码完全相同

异常场景：cbi-client connect -a localhost:50051 --fault-nack-after 3
  └── hasAnyFaultFlag(cmd) → true（--fault-nack-after 的 Changed == true）
  └── NewFaultInjectorOrNil(cmd) → 创建 injector
  └── injector != nil
  └── handler.SetAckTimeout() → 被调用（注入自定义超时）
  └── setupFaultInjections() → 被调用（注册所有注入回调）
  └── 注入点生效
```

### 阶段安排

正常场景保证机制的实现在**阶段一（1.1）**中与 `FaultConfig` 同时完成，因为 `hasAnyFaultFlag` 是保证机制的核心入口，必须优先就位。

### 单元测试补充（阶段一 1.3 追加）

```go
func TestHasAnyFaultFlag_None(t *testing.T) {
    cmd := &cobra.Command{}
    cmd.Flags().Int("fault-ack-timeout", 0, "")
    cmd.Flags().Int("fault-delay", 0, "")

    // 不传递任何参数时，应返回 false
    if hasAnyFaultFlag(cmd) {
        t.Error("expected false when no fault flags changed")
    }
}

func TestHasAnyFaultFlag_OneChanged(t *testing.T) {
    cmd := &cobra.Command{}
    cmd.Flags().Int("fault-ack-timeout", 0, "")
    cmd.Flags().String("fault-scenario", "", "")

    // 模拟传递 --fault-ack-timeout 100
    cmd.Flags().Set("fault-ack-timeout", "100")

    if !hasAnyFaultFlag(cmd) {
        t.Error("expected true when at least one fault flag is set")
    }
}

func TestHasAnyFaultFlag_DefaultValue(t *testing.T) {
    cmd := &cobra.Command{}
    cmd.Flags().Int("fault-ack-timeout", 0, "")

    // 传递 --fault-ack-timeout 0（默认值），Changed 仍为 false
    cmd.Flags().Set("fault-ack-timeout", "0")

    if hasAnyFaultFlag(cmd) {
        t.Error("expected false when fault flag set to default value")
    }
}

func TestNewFaultInjectorOrNil_Nil(t *testing.T) {
    cmd := &cobra.Command{}
    cmd.Flags().Int("fault-ack-timeout", 0, "")
    // 不传参数

    inj, err := NewFaultInjectorOrNil(cmd)
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    if inj != nil {
        t.Error("expected nil injector when no fault flags")
    }
}

func TestNewFaultInjectorOrNil_Active(t *testing.T) {
    cmd := &cobra.Command{}
    cmd.Flags().Int("fault-ack-timeout", 0, "")
    cmd.Flags().Set("fault-ack-timeout", "500")

    inj, err := NewFaultInjectorOrNil(cmd)
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    if inj == nil {
        t.Error("expected non-nil injector when fault flag is set")
    }
    if inj.GetAckTimeout() != 500*time.Millisecond {
        t.Errorf("expected 500ms, got %v", inj.GetAckTimeout())
    }
}
```

### 正常模式保证验证测试（阶段四 4.0）

```bash
# 4.0.1 验证零参数时 injector 为 nil
# 在 runConnect 开头增加调试输出（仅测试用，上线前删除）：
#   fmt.Printf("DEBUG: injector=%v\n", injector)
# 零参数运行时应为：DEBUG: injector=<nil>

# 4.0.2 编译后正常运行，无任何 [FAULT] 字样日志
cbi-client connect -a localhost:50051 &
sleep 5
# 确认日志中无 "[FAULT]" 字符串

# 4.0.3 确认零参数时 ACK 超时仍为默认 490ms
# 通过日志时间戳计算 ACK 回复间隔
```

### 风险说明

1. **Cobra 的 `Changed` 行为**：只有用户显式传递参数时 `f.Changed == true`，配置文件中的默认值**不会**触发注入。这符合预期（配置文件设置的是"默认行为"，不等于启用故障注入）。
2. **预定义场景参数 `--fault-scenario`**：当用户指定 `--fault-scenario xxx` 时，该参数的 `Changed == true`，会触发 `hasAnyFaultFlag` 返回 true，从而创建 injector。
3. **注入方法均为值 receiver 而非指针 receiver**：`FaultInjector` 使用值 receiver 定义所有方法，这样即使 injector 为 nil 指针，调用 `inj.ShouldBlockDC2()` 也不会 panic（Go 的 nil receiver 行为），但更安全的做法是在 `connect.go` 调用前检查 `injector != nil`。

---

## 阶段一：基础设施

### 1.1 新增 `cmd/cbi-client/fault/config.go`

**目标**：定义 `FaultConfig` 结构体，提供 `NewFaultConfig()` 和 `RegisterFlags()` 方法。

**实现**：

```go
// cmd/cbi-client/fault/config.go
package fault

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

func NewFaultConfig() *FaultConfig {
    return &FaultConfig{
        AckTimeout: 0,   // 0 表示使用默认值 490
        ReplyDelay: 0,   // 0 表示使用默认值 10
    }
}

func (fc *FaultConfig) RegisterFlags(cmd *cobra.Command) {
    cmd.Flags().IntVar(&fc.AckTimeout, "fault-ack-timeout", 0, "ACK timeout in ms (0=default 490)")
    cmd.Flags().IntVar(&fc.ReplyDelay, "fault-delay", 0, "Reply delay in ms (0=default 10ms)")
    cmd.Flags().BoolVar(&fc.ReplyDrop, "fault-reply-drop", false, "Drop all replies")
    cmd.Flags().IntVar(&fc.RandomDrop, "fault-random-drop", 0, "Random drop probability 0-100%")
    cmd.Flags().IntVar(&fc.SeqSkip, "fault-seq-skip", 0, "Skip seq every N frames")
    cmd.Flags().BoolVar(&fc.SeqStuck, "fault-seq-stuck", false, "Fix seq at current value")
    cmd.Flags().BoolVar(&fc.SeqMismatch, "fault-seq-mismatch", false, "Offset ackSeq")
    cmd.Flags().IntVar(&fc.NackAfter, "fault-nack-after", 0, "Send NACK after N frames received")
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

**验证方法**：
```bash
# 编译不报错
go build ./cmd/cbi-client

# 验证参数已注册
./cbi-client connect --help | grep fault
```

---

### 1.2 新增 `cmd/cbi-client/fault/injector.go`

**目标**：定义 `FaultInjector` 结构体，包含 `FaultStats` 和 `InjectionResult`，实现骨架方法。

**实现**：

```go
// cmd/cbi-client/fault/injector.go
package fault

import (
    "math/rand"
    "sync"
    "sync/atomic"
    "time"

    "cbi-simulator/internal/protocol"
)

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

type InjectionResult struct {
    Block   bool          // true = 拦截该帧，不发送/不处理
    Nack    bool          // true = 替换为 NACK
    Verror  bool          // true = 替换为 VERROR
    Delay   time.Duration // 额外的延时
    Corrupt bool          // true = 篡改数据内容
}

type FaultInjector struct {
    cfg       *FaultConfig
    stats     *FaultStats
    seqSent   int64 // 已发送帧计数（用于 seq-skip / nack-after）
    skipSeq   bool  // 下次是否跳过序号
    seqMu     sync.Mutex
    rand      *rand.Rand
}

func NewFaultInjector(cfg *FaultConfig) *FaultInjector {
    return &FaultInjector{
        cfg:    cfg,
        stats:  &FaultStats{},
        seqMu:  sync.Mutex{},
        rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (fi *FaultInjector) Stats() *FaultStats {
    return fi.stats
}

// === BeforeSend：发送帧前篡改（注入点 E）===

func (fi *FaultInjector) BeforeSend(frame *protocol.Frame) {
    // WrongVersion: 篡改版本号
    if fi.cfg.WrongVersion {
        frame.Version = 0x10 // 错误版本 0x10，正常为 0x11
    }

    // ExtraData: 篡改数据长度字段
    if fi.cfg.ExtraData && len(frame.Data) > 0 {
        frame.DataLength = uint16(len(frame.Data) + 10)
    }
}

// AfterSend：发送后更新序号状态
func (fi *FaultInjector) AfterSend(frame *protocol.Frame) {
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
    }
}

// ShouldSkipSeq：检查是否应跳过序号（由 FrameHandler.SendDataFrame 调用）
func (fi *FaultInjector) ShouldSkipSeq() bool {
    fi.seqMu.Lock()
    defer fi.seqMu.Unlock()
    if fi.skipSeq {
        fi.skipSeq = false
        return true
    }
    return false
}

// === BeforeRecv：接收帧后、序号检查前（注入点 A）===

func (fi *FaultInjector) BeforeRecv(frame *protocol.Frame) InjectionResult {
    result := InjectionResult{}

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

func (fi *FaultInjector) AfterRecvCheck(frame *protocol.Frame) {
    if !fi.cfg.CorruptData || len(frame.Data) == 0 {
        return
    }
    // 随机损坏一个字节
    pos := fi.rand.Intn(len(frame.Data))
    frame.Data[pos] ^= 0xFF
    atomic.AddInt64(&fi.stats.CorruptedFrames, 1)
}

// === 回调辅助方法 ===

func (fi *FaultInjector) ShouldReplyDrop() bool {
    if !fi.cfg.ReplyDrop {
        return false
    }
    atomic.AddInt64(&fi.stats.ReplyDropped, 1)
    return true
}

func (fi *FaultInjector) ShouldBlockDC2() bool {
    return fi.cfg.BlockDC2
}

func (fi *FaultInjector) ShouldSendVerrorOnDC2() bool {
    return fi.cfg.Verror
}

func (fi *FaultInjector) GetReplyDelay() time.Duration {
    if fi.cfg.ReplyDelay > 0 {
        return time.Duration(fi.cfg.ReplyDelay) * time.Millisecond
    }
    return 10 * time.Millisecond // defaultDelay
}

func (fi *FaultInjector) GetAckTimeout() time.Duration {
    if fi.cfg.AckTimeout > 0 {
        return time.Duration(fi.cfg.AckTimeout) * time.Millisecond
    }
    return 490 * time.Millisecond
}

func (fi *FaultInjector) ShouldSendNackAfter() bool {
    if fi.cfg.NackAfter == 0 {
        return false
    }
    sent := atomic.LoadInt64(&fi.seqSent)
    return sent >= int64(fi.cfg.NackAfter)
}

func (fi *FaultInjector) ShouldEmptyData() bool {
    return fi.cfg.EmptyData
}

func (fi *FaultInjector) ShouldDisconnect() bool {
    return fi.cfg.DisconnectAfter > 0
}

func (fi *FaultInjector) GetDisconnectAfter() time.Duration {
    return time.Duration(fi.cfg.DisconnectAfter) * time.Second
}

func (fi *FaultInjector) ShouldReconnectLoop() bool {
    return fi.cfg.ReconnectLoop
}

func (fi *FaultInjector) RecordDisconnect() {
    atomic.AddInt64(&fi.stats.DisconnectTrigger, 1)
}
```

---

### 1.3 单元测试 `cmd/cbi-client/fault/config_test.go`

```go
package fault

import (
    "testing"

    "github.com/spf13/cobra"
)

func TestFaultConfig_Defaults(t *testing.T) {
    cfg := NewFaultConfig()
    if cfg.AckTimeout != 0 {
        t.Errorf("expected AckTimeout=0, got %d", cfg.AckTimeout)
    }
    if cfg.ReplyDelay != 0 {
        t.Errorf("expected ReplyDelay=0, got %d", cfg.ReplyDelay)
    }
    if cfg.RandomDrop != 0 {
        t.Errorf("expected RandomDrop=0, got %d", cfg.RandomDrop)
    }
}

func TestFaultConfig_RegisterFlags(t *testing.T) {
    cfg := NewFaultConfig()
    cmd := &cobra.Command{}
    cfg.RegisterFlags(cmd)

    flags := []string{
        "fault-ack-timeout", "fault-delay", "fault-reply-drop",
        "fault-random-drop", "fault-seq-skip", "fault-seq-stuck",
        "fault-seq-mismatch", "fault-nack-after", "fault-nack-random",
        "fault-verror", "fault-block-dc2", "fault-wrong-version",
        "fault-corrupt-data", "fault-empty-data", "fault-extra-data",
        "fault-disconnect-after", "fault-reconnect-loop",
    }

    for _, name := range flags {
        if !cmd.Flags().Lookup(name).Changed {
            t.Errorf("flag %s not registered", name)
        }
    }
}
```

**验证方法**：
```bash
go test ./cmd/cbi-client/fault/... -v
```

---

## 阶段二：核心注入逻辑

### 2.1 注入点 E：BeforeSend / AfterSend

**已包含在 1.2 的 `FaultInjector.BeforeSend` 和 `AfterSend` 中。**

需要在 `FrameHandler.SendDataFrame()` 中调用，详见阶段三 3.3。

**验证方法**：传递 `--fault-wrong-version`，抓包验证版本号为 0x10。

---

### 2.2 注入点 A：BeforeRecv

**已包含在 1.2 的 `FaultInjector.BeforeRecv` 中。**

需要在 `client.SetOnFrameReceived` 回调中调用，详见阶段三 3.2。

**验证方法**：
- `--fault-random-drop 100`：CTC 侧持续报 ACK 超时
- `--fault-nack-random 50`：双方持续 NACK 重传

---

### 2.3 注入点 B：AfterRecvCheck

**已包含在 1.2 的 `FaultInjector.AfterRecvCheck` 中。**

需要在 `client.SetOnFrameReceived` 中序号检查后调用，详见阶段三 3.2。

**验证方法**：传递 `--fault-corrupt-data`，用日志分析工具验证帧内容异常。

---

### 2.4 连接类注入

**实现位置**：`runConnect` 中的断连定时器和重连循环逻辑。

详见阶段三 3.2。

**验证方法**：
```bash
# 5秒后断开，观察日志
cbi-client connect --fault-disconnect-after 5

# 5秒断开后自动重连，反复观察
cbi-client connect --fault-disconnect-after 5 --fault-reconnect-loop
```

---

### 2.5 单元测试 `cmd/cbi-client/fault/injector_test.go`

```go
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
        Type:       protocol.SDI,
        Data:       make([]byte, len(originalData)),
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
    if fi.GetAckTimeout() != 2000*time.Millisecond {
        t.Errorf("expected 2000ms, got %v", fi.GetAckTimeout())
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
    if fi.GetReplyDelay() != 500*time.Millisecond {
        t.Errorf("expected 500ms, got %v", fi.GetReplyDelay())
    }
}
```

---

## 阶段三：与 connect.go 集成

### 3.1 修改 `cmd/cbi-client/connect.go` - 注册 Flag 并创建 Injector

**变更 1**：新增导入

```go
import (
    "cbi-simulator/fault"
)
```

**变更 2**：`init()` 函数中注册 Flag

```go
func init() {
    rootCmd.AddCommand(connectCmd)
    connectCmd.Flags().StringVarP(&connectAddress, "address", "a", "localhost:50051", "gRPC server address")
    connectCmd.Flags().IntVarP(&connectTimeout, "timeout", "t", 30, "connection timeout in seconds")
    connectCmd.Flags().StringVarP(&logDir, "log-dir", "l", "logs", "log directory path")
    connectCmd.Flags().StringVarP(&configDir, "config", "c", "configs", "config directory path")
    // 新增：注册故障注入参数
    faultConfig := fault.NewFaultConfig()
    faultConfig.RegisterFlags(connectCmd)
}
```

注意：由于 `faultConfig` 需要在 `runConnect` 中使用，将其作为包级变量或通过 context 传递。此处建议在 `runConnect` 函数开头创建。

**变更 3**：`runConnect` 函数开头创建 Injector

```go
func runConnect(cmd *cobra.Command, args []string) {
    // 从 cobra Command 中获取故障配置（通过 Flag 的 Changed 状态判断是否启用）
    cfg, err := fault.NewFaultConfigFromCommand(cmd)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to parse fault config: %v\n", err)
        os.Exit(1)
    }
    injector := fault.NewFaultInjector(cfg)

    // 打印启用的故障注入配置
    if cfg.HasActiveFaults() {
        fmt.Printf("[FAULT INJECTION ENABLED] %s\n", cfg.String())
        fmt.Printf("[FAULT STATS] %s\n", injector.Stats().String())
    }
    // ...
}
```

为此需要新增 `NewFaultConfigFromCommand` 和 `HasActiveFaults`、`String` 方法到 `fault/config.go`：

```go
func NewFaultConfigFromCommand(cmd *cobra.Command) (*FaultConfig, error) {
    cfg := NewFaultConfig()
    fs := cmd.Flags()
    cfg.AckTimeout, _ = fs.GetInt("fault-ack-timeout")
    cfg.ReplyDelay, _ = fs.GetInt("fault-delay")
    cfg.ReplyDrop, _ = fs.GetBool("fault-reply-drop")
    cfg.RandomDrop, _ = fs.GetInt("fault-random-drop")
    cfg.SeqSkip, _ = fs.GetInt("fault-seq-skip")
    cfg.SeqStuck, _ = fs.GetBool("fault-seq-stuck")
    cfg.SeqMismatch, _ = fs.GetBool("fault-seq-mismatch")
    cfg.NackAfter, _ = fs.GetInt("fault-nack-after")
    cfg.NackRandom, _ = fs.GetInt("fault-nack-random")
    cfg.Verror, _ = fs.GetBool("fault-verror")
    cfg.BlockDC2, _ = fs.GetBool("fault-block-dc2")
    cfg.WrongVersion, _ = fs.GetBool("fault-wrong-version")
    cfg.CorruptData, _ = fs.GetBool("fault-corrupt-data")
    cfg.EmptyData, _ = fs.GetBool("fault-empty-data")
    cfg.ExtraData, _ = fs.GetBool("fault-extra-data")
    cfg.DisconnectAfter, _ = fs.GetInt("fault-disconnect-after")
    cfg.ReconnectLoop, _ = fs.GetBool("fault-reconnect-loop")
    return cfg, nil
}

func (fc *FaultConfig) HasActiveFaults() bool {
    return fc.AckTimeout > 0 || fc.ReplyDelay > 0 || fc.ReplyDrop ||
        fc.RandomDrop > 0 || fc.SeqSkip > 0 || fc.SeqStuck ||
        fc.SeqMismatch || fc.NackAfter > 0 || fc.NackRandom > 0 ||
        fc.Verror || fc.BlockDC2 || fc.WrongVersion ||
        fc.CorruptData || fc.EmptyData || fc.ExtraData ||
        fc.DisconnectAfter > 0 || fc.ReconnectLoop
}

func (fc *FaultConfig) String() string {
    var parts []string
    if fc.AckTimeout > 0 {
        parts = append(parts, fmt.Sprintf("ack-timeout=%dms", fc.AckTimeout))
    }
    if fc.ReplyDelay > 0 {
        parts = append(parts, fmt.Sprintf("delay=%dms", fc.ReplyDelay))
    }
    if fc.ReplyDrop {
        parts = append(parts, "reply-drop")
    }
    if fc.RandomDrop > 0 {
        parts = append(parts, fmt.Sprintf("random-drop=%d%%", fc.RandomDrop))
    }
    if fc.SeqSkip > 0 {
        parts = append(parts, fmt.Sprintf("seq-skip=%d", fc.SeqSkip))
    }
    if fc.SeqStuck {
        parts = append(parts, "seq-stuck")
    }
    if fc.SeqMismatch {
        parts = append(parts, "seq-mismatch")
    }
    if fc.NackAfter > 0 {
        parts = append(parts, fmt.Sprintf("nack-after=%d", fc.NackAfter))
    }
    if fc.NackRandom > 0 {
        parts = append(parts, fmt.Sprintf("nack-random=%d%%", fc.NackRandom))
    }
    if fc.Verror {
        parts = append(parts, "verror")
    }
    if fc.BlockDC2 {
        parts = append(parts, "block-dc2")
    }
    if fc.WrongVersion {
        parts = append(parts, "wrong-version")
    }
    if fc.CorruptData {
        parts = append(parts, "corrupt-data")
    }
    if fc.EmptyData {
        parts = append(parts, "empty-data")
    }
    if fc.ExtraData {
        parts = append(parts, "extra-data")
    }
    if fc.DisconnectAfter > 0 {
        parts = append(parts, fmt.Sprintf("disconnect-after=%ds", fc.DisconnectAfter))
    }
    if fc.ReconnectLoop {
        parts = append(parts, "reconnect-loop")
    }
    return strings.Join(parts, ", ")
}
```

同样需要为 `FaultStats` 添加 `String()` 方法。

---

### 3.2 修改 `cmd/cbi-client/connect.go` - 注入逻辑

#### 3.2.1 `SetAckTimeout` 配置

```go
// 在 client.Connect() 成功后
handler := client.GetHandler()
handler.SetAckTimeout(injector.GetAckTimeout())
```

#### 3.2.2 `onDC2` 回调（注入点 C）

将原 `onDC2` 替换为：

```go
handler.OnDC2(func(frame *protocol.Frame) {
    fmt.Printf("Received DC2: connection request (seq=%d)\n", frame.SendSeq)

    // 重置状态
    cbiState.mu.Lock()
    cbiState.ackCount = 0
    cbiState.roleState = 0x55
    cbiState.controlMode = 0xAA
    cbiState.mu.Unlock()

    // 故障注入：阻断 DC3
    if injector.ShouldBlockDC2() {
        fmt.Println("[FAULT] DC2 blocked, no DC3 reply")
        return
    }

    // 故障注入：回复 VERROR
    if injector.ShouldSendVerrorOnDC2() {
        fmt.Println("[FAULT] Sending VERROR on DC2")
        if err := handler.SendVERROR(); err != nil {
            fmt.Fprintf(os.Stderr, "Failed to send VERROR: %v\n", err)
        }
        handler.Disconnect()
        return
    }

    // 正常流程：延时后回复 DC3
    time.Sleep(injector.GetReplyDelay())
    if err := handler.SendDC3(); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to send DC3: %v\n", err)
    } else {
        fmt.Printf("Sent DC3 (seq initialized to 1)\n")
    }
})
```

#### 3.2.3 `onRSR` 回调（注入点 D）

```go
handler.OnRSR(func(frame *protocol.Frame) {
    // 故障注入：ReplyDrop
    if injector.ShouldReplyDrop() {
        fmt.Println("[FAULT] RSR reply dropped")
        return
    }
    if injector.ShouldEmptyData() {
        fmt.Println("[FAULT] RSR sending empty data")
    }

    cbiState.mu.Lock()
    role := cbiState.roleState
    mode := cbiState.controlMode
    cbiState.mu.Unlock()

    time.Sleep(injector.GetReplyDelay())

    replyData := []byte{role, mode}
    if injector.ShouldEmptyData() {
        replyData = nil
    }

    if err := handler.SendDataFrame(protocol.RSR, func() []byte {
        return replyData
    }); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to send RSR: %v\n", err)
    } else {
        fmt.Printf("Sent RSR (role=0x%02X, mode=0x%02X)\n", role, mode)
    }
})
```

#### 3.2.4 `onACK` 回调（注入点 D）

在 `handler.OnACK` 中，原来的 `switch` 分支前插入：

```go
handler.OnACK(func(frame *protocol.Frame) {
    cbiState.mu.Lock()
    cbiState.ackCount++
    count := cbiState.ackCount
    cbiState.mu.Unlock()

    fmt.Printf("Received ACK (count=%d, ackSeq=%d)\n", count, frame.AckSeq)

    // 故障注入：阻断回复
    if injector.ShouldReplyDrop() {
        fmt.Println("[FAULT] ACK reply dropped")
        return
    }

    // 故障注入：NackAfter
    if injector.ShouldSendNackAfter() {
        fmt.Println("[FAULT] Injecting NACK instead of reply")
        handler.SendNACK()
        return
    }

    // 故障注入：空数据
    emptyData := injector.ShouldEmptyData()

    // ... 原有 switch 分支中的发送逻辑 ...
    // 在每个 SendDataFrame 的 buildData 中：
    //   if emptyData { return nil }

    switch {
    case count == 1:
        fmt.Printf("ACK count=1: sending SDI\n")
        data := cbiState.GenerateSDIData()
        if emptyData {
            data = nil
        }
        if err := handler.SendDataFrame(protocol.SDI, func() []byte {
            return data
        }); err != nil {
            fmt.Fprintf(os.Stderr, "Failed to send SDI: %v\n", err)
        }
    // ... 其他 case ...
    }
})
```

#### 3.2.5 `client.SetOnFrameReceived` 回调（注入点 A/B）

替换原回调：

```go
client.SetOnFrameReceived(func(frame *protocol.Frame) {
    // 注入点 A：丢帧 / 随机 NACK
    result := injector.BeforeRecv(frame)
    if result.Block {
        fmt.Printf("[FAULT] Frame %s dropped\n", frame.Type)
        return
    }
    if result.Nack {
        fmt.Printf("[FAULT] Frame %s replaced with NACK\n", frame.Type)
        handler.SendNACK()
        return
    }

    // 正常记录日志
    frameData := protocol.FrameToBytes(frame)
    frameLogger.LogFrameRecv(byte(frame.Type), frameData)
    fmt.Printf("Received frame: %s (seq=%d, ack=%d)\n", frame.Type, frame.SendSeq, frame.AckSeq)

    // 注入点 B：序号检查后数据损坏（在 handler.HandleFrame 内部处理）
})
```

#### 3.2.6 `client.SetOnFrameSent` 回调（注入点 E）

新增或修改 `SetOnFrameSent`：

```go
client.SetOnFrameSent(func(frame *protocol.Frame) {
    // 注入点 E：发送前篡改
    injector.BeforeSend(frame)

    frameData := protocol.FrameToBytes(frame)
    frameLogger.LogFrameSend(byte(frame.Type), frameData)

    // 注入点 E：发送后更新序号状态
    injector.AfterSend(frame)
})
```

注意：`client.SetOnFrameSent` 如果原本不存在，需要确认 `grpc.Client` 是否支持此回调。如果不支持，BeforeSend/AfterSend 需要在 `FrameHandler.SendDataFrame()` 中直接调用（详见 3.3）。

#### 3.2.7 主动断连定时器

在 `client.Connect()` 成功后的位置添加：

```go
// 故障注入：主动断连定时器
if injector.ShouldDisconnect() {
    timer := time.NewTimer(injector.GetDisconnectAfter())
    go func() {
        <-timer.C
        fmt.Println("[FAULT] Triggering disconnect")
        injector.RecordDisconnect()
        client.Disconnect()

        if injector.ShouldReconnectLoop() {
            fmt.Println("[FAULT] Reconnecting loop...")
            for {
                time.Sleep(3 * time.Second)
                fmt.Println("[FAULT] Attempting reconnect...")
                if err := client.Connect(context.Background()); err != nil {
                    fmt.Fprintf(os.Stderr, "[FAULT] Reconnect failed: %v\n", err)
                    continue
                }
                fmt.Println("[FAULT] Reconnected successfully!")
                // 重新设置所有回调（复用同一 injector）
                setupFaultyCallbacks(client, handler, cbiState, injector, frameLogger)
                break
            }
        }
    }()
}
```

其中 `setupFaultyCallbacks` 是将所有回调设置封装成的辅助函数。

---

### 3.3 修改 `internal/grpc/frame_handler.go`

**变更 1**：新增字段

```go
type FrameHandler struct {
    client               *Client
    mu                   sync.Mutex
    sendSeq              byte
    ackSeq               byte
    ackCount             int
    roleState            byte
    controlMode          byte
    connected            bool
    verrorReceived       bool
    lastSentFrame        *protocol.Frame
    nackConsecutiveCount int
    nextFrameSendTime    time.Time
    nextFrameMu          sync.Mutex
    ackTimerDone         chan struct{}
    delayTimer           *time.Timer
    // === 新增 ===
    customAckTimeout     time.Duration  // 故障注入：自定义 ACK 超时
    injector             *fault.FaultInjector // 故障注入器
}
```

**变更 2**：新增 `SetAckTimeout` 和 `SetInjector` 方法

```go
func (h *FrameHandler) SetAckTimeout(timeout time.Duration) {
    h.nextFrameMu.Lock()
    h.customAckTimeout = timeout
    h.nextFrameMu.Unlock()
}

func (h *FrameHandler) SetInjector(inj *fault.FaultInjector) {
    h.injector = inj
}
```

**变更 3**：修改 `updateNextFrameSendTime()`

```go
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

**变更 4**：修改 `SendDataFrame()`

在 `h.sendSeq++` 之前插入序号跳跃检查：

```go
// 发送前检查是否应跳过序号（故障注入）
if h.injector != nil && h.injector.ShouldSkipSeq() {
    // 跳过序号递增，但仍然发送帧
    log.Warn("[FAULT] Skipping seq increment for this frame")
} else {
    h.sendSeq++
    if h.sendSeq == 0 {
        h.sendSeq = 1
    }
}
```

在发送前插入版本号/数据长度篡改：

```go
// 故障注入 BeforeSend（篡改版本号、数据长度等）
if h.injector != nil {
    h.injector.BeforeSend(frame)
}
```

---

### 3.4 新增 `cmd/cbi-client/fault/scenarios.go`

```go
package fault

// 预定义故障场景

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
```

同时支持命令行 `--fault-scenario` 参数：

```go
func (fc *FaultConfig) RegisterFlags(cmd *cobra.Command) {
    // ... 其他 flag ...
    cmd.Flags().StringVar(&fc.ScenarioName, "fault-scenario", "", "Predefined fault scenario")
}

func (fc *FaultConfig) ApplyScenario(name string) error {
    switch name {
    case "network-congestion":
        *fc = *NetworkCongestion()
    case "ack-timeout":
        *fc = *AckTimeout()
    case "nack-attack":
        *fc = *NackAttack()
    case "seq-disorder":
        *fc = *SeqDisorder()
    case "version-mismatch":
        *fc = *VersionMismatch()
    case "data-corruption":
        *fc = *DataCorruption()
    case "disconnect-loop":
        *fc = *DisconnectLoop()
    case "verror-disconnect":
        *fc = *VerrorDisconnect()
    case "":
        // 空场景，无操作
    default:
        return fmt.Errorf("unknown scenario: %s", name)
    }
    return nil
}
```

---

## 阶段三（修订）：与 connect.go 集成 - 正常场景保证机制

### 3.0 修改 `cmd/cbi-client/connect.go` - hasAnyFaultFlag 检测 + 条件化 Injector 创建

**目标**：在 `runConnect` 入口处通过 `hasAnyFaultFlag` 检测是否有任何 `--fault-*` 参数被传递，仅在有时才创建 `FaultInjector` 实例。

**核心变更**：`runConnect` 函数开头替换为：

```go
func runConnect(cmd *cobra.Command, args []string) {
    // 从 cobra Command 中获取 injector（无任何 --fault-* 参数时为 nil）
    injector, err := fault.NewFaultInjectorOrNil(cmd)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create fault injector: %v\n", err)
        os.Exit(1)
    }

    // injector 为 nil 时，所有注入点自动短路，正常流程不受影响
    // injector != nil 时，注册所有注入回调
    // （见 3.1 ~ 3.3）
}
```

**注意**：`init()` 函数中仍无条件执行 `faultConfig.RegisterFlags(connectCmd)`，因为需要提前注册所有 `--fault-*` 参数到 cobra，使 cobra 能够解析它们。RegisterFlags 仅完成"注册"动作，不创建任何实例，不影响运行时行为。

---

### 3.1 修改 `cmd/cbi-client/connect.go` - 在各回调中插入 nil-safe 注入逻辑

> 本节详细代码见原始设计文档 Section 3.2 ~ 3.2.7。核心原则：
> - 所有回调中的注入判断均为 `if injector != nil { ... }`
> - injector 为 nil 时，条件为 false，注入逻辑被完全跳过
> - 原版回调逻辑（记录日志、状态更新）始终执行，不受 injector 影响

**变更 1**：`init()` 函数中注册 Flag

```go
func init() {
    rootCmd.AddCommand(connectCmd)
    connectCmd.Flags().StringVarP(&connectAddress, "address", "a", "localhost:50051", "gRPC server address")
    connectCmd.Flags().IntVarP(&connectTimeout, "timeout", "t", 30, "connection timeout in seconds")
    connectCmd.Flags().StringVarP(&logDir, "log-dir", "l", "logs", "log directory path")
    connectCmd.Flags().StringVarP(&configDir, "config", "c", "configs", "config directory path")
    // 注册故障注入参数（无条件注册，仅完成 cobra.Flag 注册）
    faultConfig := fault.NewFaultConfig()
    faultConfig.RegisterFlags(connectCmd)
}
```

**变更 2**：`NewFaultConfigFromCommand`、`HasActiveFaults`、`String` 方法已在 config.go 中定义（见阶段一）。

**变更 3**：`runConnect` 中回调的 nil-safe 注入调用示例：

```go
// onDC2 回调
handler.OnDC2(func(frame *protocol.Frame) {
    // ... 正常业务逻辑（状态重置）始终执行 ...

    // 注入点 C（nil-safe）
    if injector != nil && injector.ShouldBlockDC2() {
        fmt.Println("[FAULT] DC2 blocked, no DC3 reply")
        return
    }
    if injector != nil && injector.ShouldSendVerrorOnDC2() {
        fmt.Println("[FAULT] Sending VERROR on DC2")
        handler.SendVERROR()
        handler.Disconnect()
        return
    }

    // 正常流程：延时后回复 DC3（始终执行）
    delay := 10 * time.Millisecond
    if injector != nil {
        delay = injector.GetReplyDelay() // nil-safe：injector 为 nil 时不调用
    }
    time.Sleep(delay)
    handler.SendDC3()
})

// onACK 回调中的注入点 D（nil-safe）
handler.OnACK(func(frame *protocol.Frame) {
    // ... 计数递增逻辑始终执行 ...

    // 注入点 D（nil-safe）
    if injector != nil && injector.ShouldReplyDrop() {
        fmt.Println("[FAULT] ACK reply dropped")
        return
    }
    if injector != nil && injector.ShouldSendNackAfter() {
        fmt.Println("[FAULT] Injecting NACK instead of reply")
        handler.SendNACK()
        return
    }

    // ... 原有 switch 分支始终执行，injector != nil 时在 buildData 中可返回篡改数据 ...
})

// client.SetOnFrameReceived 回调（注入点 A/B，nil-safe）
client.SetOnFrameReceived(func(frame *protocol.Frame) {
    // 注入点 A（nil-safe）
    if injector != nil {
        result := injector.BeforeRecv(frame)
        if result.Block {
            fmt.Printf("[FAULT] Frame %s dropped\n", frame.Type)
            return
        }
        if result.Nack {
            fmt.Printf("[FAULT] Frame %s replaced with NACK\n", frame.Type)
            handler.SendNACK()
            return
        }
    }

    // 原版日志记录逻辑（始终执行）
    frameData := protocol.FrameToBytes(frame)
    frameLogger.LogFrameRecv(byte(frame.Type), frameData)
    fmt.Printf("Received frame: %s\n", frame.Type)
})
```

---

### 3.2 修改 `internal/grpc/frame_handler.go` - SetAckTimeout（nil-safe）

> 本节详细代码见原始设计文档 Section 6.3。
> 核心：`SetAckTimeout` 内部通过 `nextFrameMu` 保护对 `customAckTimeout` 的写入，
> 所有注入点方法（BeforeSend 等）内部均通过 `if h.injector == nil { return }` 短路。

**新增字段**：

```go
type FrameHandler struct {
    // ... 已有字段 ...
    // === 新增（故障注入）===
    customAckTimeout time.Duration  // 0 表示使用默认值 490ms
    injector         *fault.FaultInjector // nil 表示无故障注入
}
```

**nil-safe SetAckTimeout**：

```go
func (h *FrameHandler) SetAckTimeout(timeout time.Duration) {
    h.nextFrameMu.Lock()
    h.customAckTimeout = timeout
    h.nextFrameMu.Unlock()
}
```

**updateNextFrameSendTime 支持自定义超时**：

```go
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

**SendDataFrame 中 nil-safe 注入点 E**：

```go
func (h *FrameHandler) SendDataFrame(frameType protocol.FrameType, buildData func() []byte) error {
    // ... 前置检查 ...

    // 故障注入 BeforeSend（nil-safe）
    if h.injector != nil {
        h.injector.BeforeSend(frame)
    }

    // ... 发送逻辑 ...

    // 序号跳跃（nil-safe）
    if h.injector != nil && h.injector.ShouldSkipSeq() {
        // 跳过序号递增，但仍然发送帧
        log.Warn("[FAULT] Skipping seq increment")
    } else {
        h.sendSeq++
        if h.sendSeq == 0 {
            h.sendSeq = 1
        }
    }

    // 故障注入 AfterSend（nil-safe）
    if h.injector != nil {
        h.injector.AfterSend(frame)
    }

    return nil
}
```

---

### 3.3 新增 `cmd/cbi-client/fault/scenarios.go`

> 内容与原始设计文档 Section 6.4 一致。

---

## 阶段四：端到端测试

### 4.0 正常模式保证验证

```bash
# 4.0.1 验证零参数时 injector 为 nil
# 在 runConnect 开头增加调试输出（仅测试用，上线前删除）：
#   fmt.Printf("DEBUG: injector=%v\n", injector)
# 零参数运行时应为：DEBUG: injector=<nil>

# 4.0.2 编译后正常运行，无任何 [FAULT] 字样日志
cbi-client connect -a localhost:50051 &
sleep 5
# 确认日志中无 "[FAULT]" 字符串

# 4.0.3 确认零参数时 ACK 超时仍为默认 490ms
# 通过日志时间戳计算 ACK 回复间隔
```

### 4.1 正常模式回归测试

```bash
# 不带任何故障参数，确保正常通信不受影响
cbi-client connect -a localhost:50051 -c ./configs &
# 观察日志，确认 SDI/SDCI/ACK 交互正常
# 确认日志中无 [FAULT] 字样
```

### 4.2 各异常参数单独验证

| 序号 | 命令 | 预期结果 | 验证方式 |
|------|------|----------|----------|
| 1 | `--fault-ack-timeout 2000` | ACK 回复延时变为2秒 | CTC侧观察ACK间隔 |
| 2 | `--fault-delay 3000` | 所有回复延时3秒 | 日志中时间戳差值 |
| 3 | `--fault-reply-drop` | CTC侧持续报未收到回复 | CTC日志 |
| 4 | `--fault-random-drop 100` | 所有帧丢失 | CTC侧持续超时 |
| 5 | `--fault-seq-skip 1` | 序号每次都跳过 | 日志中 seq 序列出现 1,3,5,... |
| 6 | `--fault-seq-stuck` | 序号卡在某个值 | 日志中 seq 长期不变 |
| 7 | `--fault-nack-after 3` | 第3帧后开始发NACK | 日志中看到连续NACK |
| 8 | `--fault-nack-random 50` | 约50%概率发NACK | 统计NACK次数 |
| 9 | `--fault-verror` | 收到DC2后CTC报VERROR | CTC日志中有VERROR |
| 10 | `--fault-block-dc2` | CTC持续发DC2无响应 | CTC日志 |
| 11 | `--fault-wrong-version` | CTC报版本错误 | 日志中看到0x10版本号 |
| 12 | `--fault-empty-data` | 发送帧数据为空 | 日志中帧数据长度为0 |
| 13 | `--fault-disconnect-after 5` | 5秒后断开连接 | 日志中有 disconnect |
| 14 | `--fault-reconnect-loop` | 断开后自动重连 | 观察反复断连重连 |
| 15 | `--fault-corrupt-data` | SDI/SDCI数据损坏 | 日志分析工具验证 |

### 4.3 复合场景验证

```bash
# 复合场景：延迟 + NACK + 丢包
cbi-client connect -a localhost:50051 \
    --fault-delay 1000 \
    --fault-nack-random 20 \
    --fault-random-drop 5

# 确认三个故障同时生效，无冲突
```

---

## 实现检查清单

- [ ] `fault/config.go` 新增完成（含 hasAnyFaultFlag、NewFaultInjectorOrNil）
- [ ] `fault/config_test.go` 单元测试通过（含 hasAnyFaultFlag 专项测试）
- [ ] `fault/injector.go` 新增完成（所有方法含 nil guard）
- [ ] `fault/injector_test.go` 单元测试通过
- [ ] `frame_handler.go` 修改完成（SetAckTimeout + 注入点 + nil-safe）
- [ ] `connect.go` Flag 注册完成
- [ ] `connect.go` hasAnyFaultFlag 入口 + 条件化 Injector 创建完成
- [ ] `connect.go` 所有回调注入完成（nil-safe 判断）
- [ ] `fault/scenarios.go` 新增完成
- [ ] `go build ./cmd/cbi-client` 编译通过
- [ ] **正常模式保证验证**（阶段四 4.0）通过
- [ ] 正常模式回归测试通过
- [ ] 各单项异常测试通过
- [ ] 复合场景测试通过
- [ ] `--help` 输出包含所有 `--fault-*` 参数

---

## 附录：文件变更汇总

| 文件路径 | 操作 | 关键行 |
|----------|------|--------|
| `cmd/cbi-client/fault/config.go` | 新增 | 全部 |
| `cmd/cbi-client/fault/config_test.go` | 新增 | 全部 |
| `cmd/cbi-client/fault/injector.go` | 新增 | 全部 |
| `cmd/cbi-client/fault/injector_test.go` | 新增 | 全部 |
| `cmd/cbi-client/fault/scenarios.go` | 新增 | 全部 |
| `cmd/cbi-client/connect.go` | 修改 | `init()` ~15行；`runConnect` ~80行 |
| `internal/grpc/frame_handler.go` | 修改 | 约10行 |
