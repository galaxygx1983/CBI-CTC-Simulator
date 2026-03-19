# CBI-CTC-Simulator

铁路计算机联锁系统(CBI)与调度集中系统(CTC)通信模拟器。

## 项目结构

```
CBI-CTC-Simulator/
├── cbi-tester/        # CBI客户端模拟器
│   ├── cmd/cbi-client/  # 主程序入口
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

### CTC 模拟器（ctc-sim）
- gRPC 双向流通信
- DC2/DC3 连接建立
- 状态报告（RSR）
- 按钮控制命令（BCC）
- 时间同步（TSQ/TSD）
- 站场数据请求（SDIQ）
- 心跳机制