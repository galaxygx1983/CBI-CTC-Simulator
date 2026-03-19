# cbi-client 使用指南

## 快速启动

### 1. 启动 ctc-simulator（gRPC 服务端）

```bash
cd ctc-simulator
go run -buildvcs=false ./cmd/ctc-sim start
```

### 2. 启动 cbi-client（gRPC 客户端）

```bash
cd cbi-tester
go run -buildvcs=false ./cmd/cbi-client connect
```

## 配置文件

### 配置文件位置

默认配置文件：`configs/config.yaml`

```yaml
station:
  codebit_file: lgxtq.zl      # 码位表文件路径
  error_file: Error.sys        # 错误码文件路径
  initial_state: safe

grpc:
  address: "localhost:50051"
  timeout_ms: 30000
  insecure: true
```

### 码位表文件（.zl）

**文件格式**：
```ini
[objects]
#,设备名，索引号
#,10,0
#,D114,63
#,64,37

[zlobjects]
#,设备名，字节偏移，位偏移
#,10,1,0
#,D114,8,0
#,64,5,0
```

**设备类型自动识别**：
- `D` + 数字 → 信号机（如 D114, D6）
- 纯数字 → 道岔（如 10, 64）
- 其他 → 区段（如 DK5, 102/104G）

### 错误码文件（Error.sys）

**文件格式**（制表符分隔）：
```
错误码	中文描述
0	错误办理
1	运行表满
2	区段占用
3	敌对进路
```

## 命令行参数

### 基本参数

```bash
cbi-client connect [flags]
```

**参数**：
- `-a, --address string` - gRPC 服务器地址（默认：localhost:50051）
- `-t, --timeout int` - 连接超时（秒）（默认：30）
- `-c, --config string` - 配置文件路径
- `--load-codebit` - 加载码位表（默认：true）
- `--load-error-sys` - 加载错误码表（默认：true）

### 使用示例

#### 使用默认配置

```bash
go run -buildvcs=false ./cmd/cbi-client connect
```

#### 指定配置文件

```bash
go run -buildvcs=false ./cmd/cbi-client connect --config configs/config.yaml
```

#### 连接到远程服务器

```bash
go run -buildvcs=false ./cmd/cbi-client connect -a 192.168.1.100:50051 -t 60
```

#### 禁用配置文件加载

```bash
go run -buildvcs=false ./cmd/cbi-client connect --load-codebit=false --load-error-sys=false
```

## 功能说明

### 自动加载配置文件

启动时自动加载：
1. **码位表（lgxtq.zl）** - 包含所有设备的索引和位置信息
2. **错误码（Error.sys）** - 包含错误码和中文描述

### 模拟数据发送

连接成功后，cbi-client 会自动：

1. **发送 DC2** - 连接请求帧
2. **接收 DC3** - 连接确认帧
3. **发送 RSR** - 状态报告帧（主机模式）
4. **接收 SDI/SDCI** - 站场数据帧
5. **响应 TSQ** - 时间同步请求
6. **发送 ACK** - 心跳帧（每 500ms）
7. **发送 BCC** - 按钮控制命令（每 10-60 秒随机间隔）

### 码位表集成

加载码位表后，BCC 命令会：
- 从码位表中**随机选择设备**
- 随机选择**控制命令**（定位/反位/取消）
- 显示设备名称和索引

**示例输出**：
```
Loading codebit table: lgxtq.zl...
Loaded 287 devices from codebit table
Loading error codes: Error.sys...
Loaded 43 error codes
Connecting to localhost:50051...
Connected successfully!
Sent DC2 frame (seq=1)
Received DC3: connection confirmed (seq=2)
Sent RSR frame (seq=3)
Sent BCC frame: device=D114 (idx=63), cmd=定位 (seq=4)
Sent BCC frame: device=64 (idx=37), cmd=反位 (seq=5)
```

## 通信流程

```
cbi-client                    ctc-simulator
    |                              |
    |-------- DC2 (连接请求) ------>|
    |                              |
    |<------- DC3 (连接确认) -------|
    |                              |
    |-------- RSR (状态报告) ------>|
    |                              |
    |<------- SDI (站场数据) -------|
    |                              |
    |-------- ACK (确认) ---------->|
    |                              |
    |<------- TSQ (时间同步) -------|
    |                              |
    |-------- TSD (时间数据) ------>|
    |                              |
    |~~~~~~~~ ACK (心跳 500ms) ~~~~>|
    |                              |
    |~~~~~~~ BCC (控制命令) ~~~~~~~>|
    |    (10-60 秒随机间隔)          |
```

## 故障排除

### 配置文件未找到

```
No config file found, using defaults
```

**解决**：确保配置文件在以下位置之一：
- 当前目录
- `./configs/config.yaml`
- `/etc/cbi-sim/config.yaml`

### 码位表加载失败

```
Warning: Failed to load codebit table: open lgxtq.zl: no such file or directory
Continuing without codebit table...
```

**解决**：
1. 检查文件路径是否正确
2. 确认文件存在：`ls configs/lgxtq.zl`
3. 或使用 `--load-codebit=false` 禁用加载

### 连接失败

```
Failed to connect: dial failed: ...
```

**解决**：
1. 确认 ctc-simulator 已启动
2. 检查地址是否正确：`--address localhost:50051`
3. 检查防火墙设置

## API 使用（编程方式）

```go
package main

import (
    "cbi-simulator/internal/grpc"
    "cbi-simulator/internal/station"
)

// 加载码位表
codebitTable, err := station.LoadCodebitTable("lgxtq.zl")

// 加载错误码
errorTable, err := station.LoadErrorCodeTable("Error.sys")

// 创建客户端
client, err := grpc.NewClient("localhost:50051")

// 连接并发送帧...
```

## 相关文件

| 文件 | 说明 |
|------|------|
| `configs/config.yaml` | 主配置文件 |
| `configs/lgxtq.zl` | 码位表文件 |
| `configs/Error.sys` | 错误码文件 |
| `internal/station/codebit.go` | 码位表加载器 |
| `internal/station/error_codes.go` | 错误码加载器 |
| `cmd/cbi-client/connect.go` | 客户端主逻辑 |
