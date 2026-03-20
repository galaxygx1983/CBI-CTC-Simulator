# CBI 故障注入使用指南

| 版本 | 日期       | 作者 | 说明 |
|------|------------|------|------|
| v1.0 | 2026-03-20 |      | 初稿 |

---

## 概述

CBI 客户端支持通过命令行参数注入各类通信故障，用于测试 CTC 系统的容错能力。所有故障参数均以 `--fault-` 为前缀，支持组合使用。

**重要特性**：不传递任何 `--fault-*` 参数时，客户端行为与原版完全相同，零侵入设计。

---

## 快速开始

### 正常模式

```bash
# 不带任何故障参数，正常运行
cbi-client connect -a localhost:50051
```

### 使用预定义场景

```bash
# 网络拥塞场景
cbi-client connect -a localhost:50051 --fault-scenario network-congestion

# 断连重连场景
cbi-client connect -a localhost:50051 --fault-scenario disconnect-loop
```

### 组合多个故障参数

```bash
# 延时 + 随机丢包
cbi-client connect -a localhost:50051 --fault-delay 1000 --fault-random-drop 10

# 复杂场景：延时 + NACK + 丢包
cbi-client connect -a localhost:50051 \
    --fault-delay 1000 \
    --fault-nack-random 20 \
    --fault-random-drop 5
```

---

## 参数详解

### 时序类参数

#### `--fault-ack-timeout <ms>`

设置 ACK 超时时间（默认 490ms）。

**用途**：模拟网络延迟或 CTC 响应慢的场景。

**示例**：
```bash
# 设置 ACK 超时为 2 秒
cbi-client connect -a localhost:50051 --fault-ack-timeout 2000
```

**预期效果**：CTC 侧可能因超时而重发帧。

---

#### `--fault-delay <ms>`

设置所有回复的统一延时（默认 10ms）。

**用途**：模拟处理延迟。

**示例**：
```bash
# 所有回复延时 500ms
cbi-client connect -a localhost:50051 --fault-delay 500
```

---

#### `--fault-reply-drop`

丢弃所有回复，不发送任何帧。

**用途**：测试 CTC 的超时重传机制。

**示例**：
```bash
cbi-client connect -a localhost:50051 --fault-reply-drop
```

**预期效果**：CTC 侧持续等待回复，可能触发超时重传。

---

#### `--fault-random-drop <percent>`

随机丢帧概率（0-100%）。

**用途**：模拟不稳定的网络环境。

**示例**：
```bash
# 20% 概率丢弃接收到的帧
cbi-client connect -a localhost:50051 --fault-random-drop 20
```

---

### 序号类参数

#### `--fault-seq-skip <n>`

每发送 N 帧跳过 1 个序号。

**用途**：测试序号错乱检测能力。

**示例**：
```bash
# 每 3 帧跳过 1 个序号（序号序列：1, 2, 3, 5, 6, 7, 9...）
cbi-client connect -a localhost:50051 --fault-seq-skip 3
```

**预期效果**：CTC 侧检测到序号跳跃，可能发送 NACK 或断开连接。

---

#### `--fault-seq-stuck`

发送序号卡住不递增。

**用途**：测试重复帧检测能力。

**示例**：
```bash
cbi-client connect -a localhost:50051 --fault-seq-stuck
```

---

#### `--fault-seq-mismatch`

确认序号故意偏移。

**用途**：测试确认序号不匹配场景。

---

### 帧类型类参数

#### `--fault-nack-after <n>`

收到 N 帧后开始回复 NACK。

**用途**：模拟持续否定应答。

**示例**：
```bash
# 收到 3 帧后开始回复 NACK
cbi-client connect -a localhost:50051 --fault-nack-after 3
```

**预期效果**：CTC 侧收到 NACK 后重传，CBI 继续回复 NACK，可能导致连接断开。

---

#### `--fault-nack-random <percent>`

随机 NACK 概率（0-100%）。

**用途**：模拟间歇性通信错误。

**示例**：
```bash
# 30% 概率回复 NACK
cbi-client connect -a localhost:50051 --fault-nack-random 30
```

---

#### `--fault-verror`

收到 DC2（连接请求）后回复 VERROR。

**用途**：模拟版本不兼容。

**示例**：
```bash
cbi-client connect -a localhost:50051 --fault-verror
```

**预期效果**：CTC 侧收到 VERROR 后断开连接。

---

#### `--fault-block-dc2`

收到 DC2 后不回复 DC3。

**用途**：测试连接建立超时。

**示例**：
```bash
cbi-client connect -a localhost:50051 --fault-block-dc2
```

**预期效果**：CTC 侧等待 DC3 超时，可能重发 DC2 或断开。

---

#### `--fault-wrong-version`

发送帧使用错误版本号（0x10，正常为 0x11）。

**用途**：测试版本校验。

**示例**：
```bash
cbi-client connect -a localhost:50051 --fault-wrong-version
```

**预期效果**：CTC 侧检测到版本错误，发送 VERROR 并断开。

---

### 数据类参数

#### `--fault-corrupt-data`

随机损坏 SDI/SDCI 数据内容。

**用途**：测试数据完整性检测。

**示例**：
```bash
cbi-client connect -a localhost:50051 --fault-corrupt-data
```

**预期效果**：CTC 侧收到损坏数据，可能触发错误处理。

---

#### `--fault-empty-data`

发送帧数据长度为 0。

**用途**：测试空数据处理。

**示例**：
```bash
cbi-client connect -a localhost:50051 --fault-empty-data
```

---

#### `--fault-extra-data`

数据长度字段与实际数据不符。

**用途**：测试长度校验。

---

### 连接类参数

#### `--fault-disconnect-after <seconds>`

N 秒后主动断开连接。

**用途**：测试连接断开处理。

**示例**：
```bash
# 5 秒后主动断开
cbi-client connect -a localhost:50051 --fault-disconnect-after 5
```

---

#### `--fault-reconnect-loop`

断开后自动重连循环。

**用途**：测试重连机制。

**示例**：
```bash
# 5 秒断开后自动重连
cbi-client connect -a localhost:50051 \
    --fault-disconnect-after 5 \
    --fault-reconnect-loop
```

**预期效果**：每 5 秒断开后自动重连，观察 CTC 侧的重连处理。

---

## 预定义场景

### network-congestion（网络拥塞）

```bash
cbi-client connect -a localhost:50051 --fault-scenario network-congestion
```

**参数组合**：`--fault-delay 2000 --fault-random-drop 10`

**测试目的**：验证 CTC 在高延迟、丢包环境下的通信能力。

---

### ack-timeout（ACK 超时）

```bash
cbi-client connect -a localhost:50051 --fault-scenario ack-timeout
```

**参数组合**：`--fault-ack-timeout 2000`

**测试目的**：验证 CTC 的超时重传机制。

---

### nack-attack（NACK 攻击）

```bash
cbi-client connect -a localhost:50051 --fault-scenario nack-attack
```

**参数组合**：`--fault-nack-after 2`

**测试目的**：验证 CTC 在持续收到 NACK 时的处理能力。

---

### seq-disorder（序号错乱）

```bash
cbi-client connect -a localhost:50051 --fault-scenario seq-disorder
```

**参数组合**：`--fault-seq-skip 3`

**测试目的**：验证 CTC 的序号检查和恢复能力。

---

### version-mismatch（版本不匹配）

```bash
cbi-client connect -a localhost:50051 --fault-scenario version-mismatch
```

**参数组合**：`--fault-wrong-version`

**测试目的**：验证 CTC 的版本校验和 VERROR 响应。

---

### data-corruption（数据损坏）

```bash
cbi-client connect -a localhost:50051 --fault-scenario data-corruption
```

**参数组合**：`--fault-corrupt-data`

**测试目的**：验证 CTC 的数据完整性检查。

---

### disconnect-loop（断连重连）

```bash
cbi-client connect -a localhost:50051 --fault-scenario disconnect-loop
```

**参数组合**：`--fault-disconnect-after 5 --fault-reconnect-loop`

**测试目的**：验证 CTC 的连接恢复能力。

---

### verror-disconnect（VERROR 断开）

```bash
cbi-client connect -a localhost:50051 --fault-scenario verror-disconnect
```

**参数组合**：`--fault-verror`

**测试目的**：验证 CTC 收到 VERROR 后的处理。

---

## 典型测试场景

### 场景1：测试超时重传

```bash
# 设置较长的回复延时，观察 CTC 超时重传
cbi-client connect -a localhost:50051 --fault-delay 3000
```

**观察点**：
- CTC 是否在超时后重发
- 重发次数和间隔
- 序号是否正确处理

---

### 场景2：测试 NACK 连续处理

```bash
# 收到 2 帧后持续回复 NACK
cbi-client connect -a localhost:50051 --fault-nack-after 2
```

**观察点**：
- CTC 是否正确重传
- 连续 NACK 次数达到阈值后的处理（通常断开）

---

### 场景3：测试序号恢复

```bash
# 每 5 帧跳过 1 个序号
cbi-client connect -a localhost:50051 --fault-seq-skip 5
```

**观察点**：
- CTC 是否检测到序号跳跃
- 是否发送 NACK 请求重传
- 通信是否恢复正常

---

### 场景4：测试连接稳定性

```bash
# 复合故障：延迟 + 丢包 + 随机 NACK
cbi-client connect -a localhost:50051 \
    --fault-delay 500 \
    --fault-random-drop 10 \
    --fault-nack-random 15
```

**观察点**：
- 通信是否维持
- 吞吐量变化
- 错误恢复时间

---

### 场景5：测试异常断开

```bash
# 10 秒后断开，然后自动重连
cbi-client connect -a localhost:50051 \
    --fault-disconnect-after 10 \
    --fault-reconnect-loop
```

**观察点**：
- CTC 是否正确处理连接断开
- 重连后是否恢复正常通信
- 日志中断开和重连记录

---

## 日志输出

启用故障注入后，客户端会输出相关日志：

```
[FAULT INJECTION ENABLED] ack-timeout=2000ms, delay=500ms, random-drop=10%
[FAULT] Frame ACK dropped
[FAULT] Injecting NACK instead of reply
[FAULT] DC2 blocked, no DC3 reply
[FAULT STATS] dropped=5, delayed=10, corrupted=0, nack=3, seq-skip=0, disconnect=0
```

---

## 注意事项

1. **正常模式保证**：不传递任何 `--fault-*` 参数时，客户端行为与原版完全相同，不会有任何性能影响。

2. **参数组合**：多个故障参数可以组合使用，但某些组合可能导致通信无法建立：
   - `--fault-reply-drop` 与 `--fault-nack-after` 同时启用时，NACK 也会被丢弃
   - `--fault-block-dc2` 会阻止 DC3 回复，导致连接无法建立

3. **序号参数互斥**：`--fault-seq-stuck` 和 `--fault-seq-skip` 不应同时启用。

4. **版本错误不可恢复**：`--fault-wrong-version` 会导致 CTC 发送 VERROR 并断开，重连后仍会复现。

5. **生产环境禁用**：故障注入仅用于测试，切勿在生产环境使用。

---

## 相关文档

- [故障注入设计方案](fault-injection-design.md)
- [故障注入实现计划](fault-injection-implementation-plan.md)
- [CBI报文收发及通信规则说明](CBI报文收发及通信规则说明.txt)