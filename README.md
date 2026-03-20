# CBI-CTC-Simulator

铁路计算机联锁系统(CBI)与调度集中系统(CTC)通信模拟器。

## 项目结构

```
CBI-CTC-Simulator/
├── cbi-tester/        # CBI客户端模拟器
│   ├── cmd/cbi-client/  # 主程序入口
│   │   └── fault/       # 故障注入模块
│   ├── internal/        # 内部包
│   └── configs/         # 配置文件
├── ctc-simulator/     # CTC服务端模拟器
│   ├── cmd/ctc-sim/     # 主程序入口
│   ├── internal/        # 内部包
│   └── configs/         # 配置文件
└── README.md
```

## 编译

### 编译 CBI 客户端

```bash
cd cbi-tester
go build -o cbi-client.exe ./cmd/cbi-client/
```

### 编译 CTC 模拟器

```bash
cd ctc-simulator
go build -o ctc-sim.exe ./cmd/ctc-sim/
```

## 运行

### 启动 CTC 模拟器（服务端）

```bash
ctc-sim start -a :50001
```

参数说明：
- `start` - 启动服务
- `-a :50001` - 监听地址和端口

### 启动 CBI 客户端（客户端）

```bash
cbi-client connect -a :50001
```

参数说明：
- `connect` - 连接到服务端
- `-a :50001` - 服务端地址和端口

## 通信协议

详细通信协议说明请参考：[CBI报文收发及通信规则说明](cbi-tester/docs/CBI报文收发及通信规则说明.txt)

## 功能特性

### CBI 客户端（cbi-client）
- DC2/DC3 连接握手
- 数据帧序号控制
- ACK 计数器响应逻辑
- NACK 重传机制（最多5次）
- VERROR 永久断连处理
- 490ms ACK 定时器
- 站场数据生成（SDI/SDCI/FIR）
- **故障注入测试支持**（详见下方）

### CTC 模拟器（ctc-sim）
- gRPC 双向流通信
- DC2/DC3 连接建立
- 状态报告（RSR）
- 按钮控制命令（BCC）
- 时间同步（TSQ/TSD）
- 站场数据请求（SDIQ）
- 心跳机制

## 故障注入测试

CBI 客户端支持通过命令行参数模拟各类异常通信场景，用于测试 CTC 系统的容错能力。

### 快速开始

```bash
# 正常模式（无故障注入）
cbi-client connect -a localhost:50051

# 使用预定义场景
cbi-client connect -a localhost:50051 --fault-scenario network-congestion

# 组合多个故障参数
cbi-client connect -a localhost:50051 --fault-delay 1000 --fault-random-drop 10
```

### 可用参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--fault-ack-timeout` | int | 490 | ACK 超时时间(ms) |
| `--fault-delay` | int | 10 | 回复延时(ms) |
| `--fault-reply-drop` | bool | false | 丢弃所有回复 |
| `--fault-random-drop` | int | 0 | 随机丢帧概率(0-100%) |
| `--fault-seq-skip` | int | 0 | 每N帧跳过1个序号 |
| `--fault-seq-stuck` | bool | false | 序号卡住不递增 |
| `--fault-seq-mismatch` | bool | false | AckSeq 故意偏移 |
| `--fault-nack-after` | int | 0 | 收到N帧后回复NACK |
| `--fault-nack-random` | int | 0 | 随机NACK概率(0-100%) |
| `--fault-verror` | bool | false | 收到DC2后回复VERROR |
| `--fault-block-dc2` | bool | false | 收到DC2后不回复DC3 |
| `--fault-wrong-version` | bool | false | 使用错误版本号0x10 |
| `--fault-corrupt-data` | bool | false | 数据内容随机损坏 |
| `--fault-empty-data` | bool | false | 发送空数据帧 |
| `--fault-extra-data` | bool | false | 数据长度字段与实际不符 |
| `--fault-disconnect-after` | int | 0 | N秒后主动断开 |
| `--fault-reconnect-loop` | bool | false | 断开后自动重连 |
| `--fault-scenario` | string | "" | 预定义场景名称 |

### 预定义场景

| 场景名 | 说明 |
|--------|------|
| `network-congestion` | 网络拥塞（2秒延迟 + 10%丢包） |
| `ack-timeout` | ACK超时（超时时间设为2秒） |
| `nack-attack` | NACK攻击（收到2帧后开始发NACK） |
| `seq-disorder` | 序号错乱（每3帧跳过1个序号） |
| `version-mismatch` | 版本不匹配（使用错误版本号） |
| `data-corruption` | 数据损坏（SDI/SDCI数据内容异常） |
| `disconnect-loop` | 断连重连（5秒后断开，自动重连） |
| `verror-disconnect` | VERROR断开（收到DC2后主动发VERROR） |

### 典型测试场景

详细使用说明请参考：[故障注入使用指南](cbi-tester/docs/fault-injection-usage.md)

## 文档

- [CBI报文收发及通信规则说明](cbi-tester/docs/CBI报文收发及通信规则说明.txt)
- [故障注入使用指南](cbi-tester/docs/fault-injection-usage.md)
- [故障注入设计方案](cbi-tester/docs/fault-injection-design.md)
- [故障注入实现计划](cbi-tester/docs/fault-injection-implementation-plan.md)