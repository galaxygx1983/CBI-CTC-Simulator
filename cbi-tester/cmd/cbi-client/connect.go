// cmd/cbi-client/connect.go
// CBI客户端 - 彻底重构
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"cbi-simulator/internal/grpc"
	"cbi-simulator/internal/logger"
	"cbi-simulator/internal/protocol"
	"cbi-simulator/internal/station"

	"github.com/spf13/cobra"
)

var (
	connectAddress string
	connectTimeout int
	logDir         string
	configDir      string
)

func init() {
	rootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringVarP(&connectAddress, "address", "a", "localhost:50051", "gRPC server address")
	connectCmd.Flags().IntVarP(&connectTimeout, "timeout", "t", 30, "connection timeout in seconds")
	connectCmd.Flags().StringVarP(&logDir, "log-dir", "l", "logs", "log directory path")
	connectCmd.Flags().StringVarP(&configDir, "config", "c", "configs", "config directory path")
}

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to CTC gRPC server",
	Long: `Connect to CTC gRPC server and start frame communication.

Examples:
  cbi-client connect
  cbi-client connect --address localhost:50051
  cbi-client connect -a 192.168.1.100:50051 -t 60`,
	Run: runConnect,
}

// CBIState CBI客户端状态
type CBIState struct {
	mu            sync.Mutex
	ackCount      int   // ACK计数器
	roleState     byte  // 主备状态变量：0x55=主机, 0xAA=备机
	controlMode   byte  // 控制模式变量：0x55=自律控制, 0xAA=非常站控
	stationMgr    *station.StationStateManager
	errorTable    *station.ErrorCodeTable
}

// NewCBIState 创建CBI状态
func NewCBIState() *CBIState {
	return &CBIState{
		roleState:   0x55, // 默认主机
		controlMode: 0xAA, // 默认非常站控
	}
}

// LoadConfigs 加载配置文件
func (s *CBIState) LoadConfigs(configPath string) error {
	// 加载码位表
	codebitPath := filepath.Join(configPath, "lgxtq.zl")
	codebitTable, err := station.LoadCodebitTable(codebitPath)
	if err != nil {
		return fmt.Errorf("load codebit table: %w", err)
	}
	s.stationMgr = station.NewStationStateManager(codebitTable)
	fmt.Printf("Loaded %d devices from %s\n", len(codebitTable.Devices), codebitPath)

	// 加载错误码表
	errorPath := filepath.Join(configPath, "Error.sys")
	errorTable, err := station.LoadErrorCodeTable(errorPath)
	if err != nil {
		fmt.Printf("Warning: failed to load error codes: %v\n", err)
	} else {
		s.errorTable = errorTable
		fmt.Printf("Loaded %d error codes from %s\n", errorTable.GetErrorCount(), errorPath)
	}

	return nil
}

// GenerateSDIData 生成SDI完整站场数据
func (s *CBIState) GenerateSDIData() []byte {
	if s.stationMgr != nil {
		s.stationMgr.RandomizeStates()
		return s.stationMgr.BuildSDIData()
	}
	// 回退：生成随机数据
	return generateRandomData(50)
}

// GenerateSDCIData 生成SDCI增量数据
func (s *CBIState) GenerateSDCIData() []byte {
	if s.stationMgr != nil {
		return s.stationMgr.BuildSDCIData()
	}
	// 回退：生成随机数据
	return generateRandomData(3)
}

// GenerateFIRData 生成FIR故障数据
func (s *CBIState) GenerateFIRData() []byte {
	// FIR格式：故障类型(1字节) + 设备索引(2字节) + 故障码(2字节)
	data := make([]byte, 5)
	data[0] = byte(rand.Intn(10) + 1) // 故障类型1-10

	if s.stationMgr != nil && len(s.stationMgr.CodebitTable.Devices) > 0 {
		// 随机选择设备
		dev := s.stationMgr.CodebitTable.Devices[rand.Intn(len(s.stationMgr.CodebitTable.Devices))]
		data[1] = byte(dev.Index & 0xFF)
		data[2] = byte((dev.Index >> 8) & 0xFF)
	} else {
		data[1] = byte(rand.Intn(256))
		data[2] = byte(rand.Intn(256))
	}

	if s.errorTable != nil {
		errors := s.errorTable.GetAllErrors()
		if len(errors) > 0 {
			errCode := errors[rand.Intn(len(errors))]
			data[3] = byte(errCode.Code & 0xFF)
			data[4] = byte((errCode.Code >> 8) & 0xFF)
		}
	} else {
		data[3] = byte(rand.Intn(43))
		data[4] = 0
	}

	return data
}

// generateRandomData 生成随机数据
func generateRandomData(length int) []byte {
	data := make([]byte, length)
	for i := range data {
		data[i] = byte(rand.Intn(256))
	}
	return data
}

func runConnect(cmd *cobra.Command, args []string) {
	// 初始化日志文件
	logPath := filepath.Join(logDir, time.Now().Format("ZLEvents060102"))
	frameLogger, err := logger.NewLogger(&logger.Config{
		FrameLogPath: logPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer frameLogger.Close()

	// 创建客户端
	client, err := grpc.NewClient(connectAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// 创建CBI状态
	cbiState := NewCBIState()

	// 加载配置文件
	if err := cbiState.LoadConfigs(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load configs: %v\n", err)
	}

	// 获取 handler
	handler := client.GetHandler()

	// 设置帧发送回调 - 记录发送日志
	client.SetOnFrameSent(func(frame *protocol.Frame) {
		frameData := protocol.FrameToBytes(frame)
		frameLogger.LogFrameSend(byte(frame.Type), frameData)
	})

	// === 帧处理回调 ===

	// 1. DC2: 收到DC2后，初始化状态并回复DC3
	handler.OnDC2(func(frame *protocol.Frame) {
		fmt.Printf("Received DC2: connection request (seq=%d)\n", frame.SendSeq)

		// 重置状态
		cbiState.mu.Lock()
		cbiState.ackCount = 0
		cbiState.roleState = 0x55   // 主机
		cbiState.controlMode = 0xAA // 非常站控
		cbiState.mu.Unlock()

		// 延时10ms回复DC3
		time.Sleep(10 * time.Millisecond)
		if err := handler.SendDC3(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send DC3: %v\n", err)
		} else {
			fmt.Printf("Sent DC3 (seq initialized to 1)\n")
		}
	})

	// 2. RSR: 延时10ms回复RSR
	handler.OnRSR(func(frame *protocol.Frame) {
		cbiState.mu.Lock()
		role := cbiState.roleState
		mode := cbiState.controlMode
		cbiState.mu.Unlock()

		// 延时10ms回复RSR
		time.Sleep(10 * time.Millisecond)

		if err := handler.SendDataFrame(protocol.RSR, func() []byte {
			return []byte{role, mode}
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send RSR: %v\n", err)
		} else {
			fmt.Printf("Sent RSR (role=0x%02X, mode=0x%02X)\n", role, mode)
		}
	})

	// 3. ACK: 根据ACK计数器响应
	// 3.1 ACK计数器=1 → 回复SDI
	// 3.2 ACK计数器=2 → 回复ACQ
	// 3.3 ACK计数器=10 → 回复TSQ
	// 3.4 ACK计数器>10且为3的倍数 → 回复SDCI
	// 3.5 ACK计数器>10且为5的倍数 → 回复FIR
	// 注：3.6已移除，ACK通过500ms定时器机制发送
	handler.OnACK(func(frame *protocol.Frame) {
		cbiState.mu.Lock()
		cbiState.ackCount++
		count := cbiState.ackCount
		cbiState.mu.Unlock()

		fmt.Printf("Received ACK (count=%d, ackSeq=%d)\n", count, frame.AckSeq)

		switch {
		case count == 1:
			// 回复SDI（完整站场数据）
			fmt.Printf("ACK count=1: sending SDI\n")
			if err := handler.SendDataFrame(protocol.SDI, func() []byte {
				return cbiState.GenerateSDIData()
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send SDI: %v\n", err)
			}

		case count == 2:
			// 回复ACQ（请求自律控制）
			fmt.Printf("ACK count=2: sending ACQ\n")
			if err := handler.SendDataFrame(protocol.ACQ, func() []byte {
				return []byte{0x55} // 请求自律控制
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send ACQ: %v\n", err)
			}

		case count == 10:
			// 回复TSQ（时间同步请求）
			fmt.Printf("ACK count=10: sending TSQ\n")
			if err := handler.SendDataFrame(protocol.TSQ, func() []byte {
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send TSQ: %v\n", err)
			}

		case count > 10 && count%3 == 0:
			// 回复SDCI（增量数据）
			fmt.Printf("ACK count=%d: sending SDCI\n", count)
			if err := handler.SendDataFrame(protocol.SDCI, func() []byte {
				return cbiState.GenerateSDCIData()
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send SDCI: %v\n", err)
			}

		case count > 10 && count%5 == 0:
			// 回复FIR（故障报告）
			fmt.Printf("ACK count=%d: sending FIR\n", count)
			if err := handler.SendDataFrame(protocol.FIR, func() []byte {
				return cbiState.GenerateFIRData()
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send FIR: %v\n", err)
			}
		}
		// 其他情况不发送任何帧，等待500ms定时器触发ACK
	})

	// 4. ACA: 如果同意自律控制，更新控制模式，延时10ms回复ACK
	handler.OnACA(func(frame *protocol.Frame) {
		if len(frame.Data) > 0 && frame.Data[0] == 0x55 {
			cbiState.mu.Lock()
			cbiState.controlMode = 0x55 // 自律控制
			cbiState.mu.Unlock()
			fmt.Println("Control mode changed to auto (0x55)")
		}
		// 延时10ms回复ACK已在handler中处理
	})

	// 5. TSD: 延时10ms回复ACK（已在handler中处理）
	handler.OnTSD(func(frame *protocol.Frame) {
		fmt.Printf("Received TSD (len=%d)\n", frame.DataLength)
	})

	// 6. BCC: 延时10ms回复ACK（已在handler中处理）
	handler.OnBCC(func(frame *protocol.Frame) {
		fmt.Printf("Received BCC (len=%d)\n", frame.DataLength)
	})

	// 7. SDIQ: 延时10ms回复SDI
	handler.OnSDIQ(func(frame *protocol.Frame) {
		fmt.Println("Received SDIQ: station data request")
		time.Sleep(10 * time.Millisecond)
		if err := handler.SendDataFrame(protocol.SDI, func() []byte {
			return cbiState.GenerateSDIData()
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send SDI: %v\n", err)
		}
	})

	// 设置帧接收回调 - 记录日志
	client.SetOnFrameReceived(func(frame *protocol.Frame) {
		frameData := protocol.FrameToBytes(frame)
		frameLogger.LogFrameRecv(byte(frame.Type), frameData)
		fmt.Printf("Received frame: %s (seq=%d, ack=%d)\n", frame.Type, frame.SendSeq, frame.AckSeq)
	})

	// 设置错误回调
	client.SetOnError(func(err error) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	})

	// 连接服务端
	ctx := context.Background()
	fmt.Printf("Connecting to %s...\n", connectAddress)

	if err := client.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected successfully!")
	fmt.Println("Press Ctrl+C to disconnect")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nDisconnecting...")
	if err := client.Disconnect(); err != nil {
		fmt.Fprintf(os.Stderr, "Error disconnecting: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Disconnected")
}