// internal/grpc/client.go
package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "cbi-simulator/internal/pb/cbi/v1"
	"cbi-simulator/internal/protocol"
	"cbi-simulator/internal/station"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client gRPC 客户端
type Client struct {
	address string
	conn    *grpc.ClientConn
	client  pb.CBISimulatorClient
	stream  pb.CBISimulator_FrameStreamClient
	station *station.StationState
	handler *FrameHandler
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc

	connected bool

	// 回调
	onFrame func(frame *protocol.Frame)
	onError func(err error)
	onSend  func(frame *protocol.Frame)

	// 配置
	reconnectMax  int
	reconnectWait time.Duration
}

// NewClient 创建客户端
func NewClient(address string) (*Client, error) {
	c := &Client{
		address:       address,
		station:       station.NewStationState(),
		reconnectMax:  3,
		reconnectWait: time.Second,
	}
	c.handler = NewFrameHandler(c)
	return c, nil
}

// Address 返回服务端地址
func (c *Client) Address() string {
	return c.address
}

// IsConnected 返回连接状态
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Connect 连接到服务端
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	// 建立 gRPC 连接
	conn, err := grpc.NewClient(c.address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.conn = conn
	c.client = pb.NewCBISimulatorClient(conn)
	c.ctx, c.cancel = context.WithCancel(ctx)

	// 创建 FrameStream
	stream, err := c.client.FrameStream(c.ctx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("create stream failed: %w", err)
	}

	c.stream = stream
	c.connected = true

	// 启动接收协程
	go c.receiveLoop()

	log.Info("CBI client connected, waiting to receive frames from server")
	return nil
}

// Disconnect 断开连接
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false

	if c.cancel != nil {
		c.cancel()
	}

	if c.stream != nil {
		c.stream.CloseSend()
	}

	if c.conn != nil {
		c.conn.Close()
	}

	return nil
}

// receiveLoop 接收消息循环
func (c *Client) receiveLoop() {
	log.Info("receiveLoop started, waiting for frames from server")
	for {
		select {
		case <-c.ctx.Done():
			log.Info("receiveLoop: context done, exiting")
			return
		default:
		}

		// 阻塞接收帧，直到 context 取消或连接关闭
		resp, err := c.stream.Recv()
		if err != nil {
			log.Infof("receiveLoop: Recv error: %v", err)
			if c.onError != nil {
				c.onError(err)
			}
			return
		}

		log.Infof("receiveLoop: received response, frame=%v", resp.Frame)
		if resp.Frame != nil {
			frame := protoFrameToFrame(resp.Frame)
			log.Infof("receiveLoop: parsed frame: %s (seq=%d, ack=%d)", frame.Type, frame.SendSeq, frame.AckSeq)

			// 先调用回调记录接收日志（在处理之前）
			if c.onFrame != nil {
				c.onFrame(frame)
			}

			// 再使用 handler 处理帧
			if c.handler != nil {
				c.handler.HandleFrame(frame)
			}
		}
	}
}

// SendFrame 发送帧
func (c *Client) SendFrame(frame *protocol.Frame) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.stream == nil {
		return fmt.Errorf("not connected")
	}

	// 调用发送回调
	if c.onSend != nil {
		c.onSend(frame)
	}

	req := &pb.FrameRequest{
		Frame: frameToProtoFrame(frame),
	}

	return c.stream.Send(req)
}

// SetOnFrameReceived 设置帧接收回调
func (c *Client) SetOnFrameReceived(callback func(frame *protocol.Frame)) {
	c.onFrame = callback
}

// SetOnError 设置错误回调
func (c *Client) SetOnError(callback func(err error)) {
	c.onError = callback
}

// SetOnFrameSent 设置帧发送回调
func (c *Client) SetOnFrameSent(callback func(frame *protocol.Frame)) {
	c.onSend = callback
}

// GetStation 获取站场状态
func (c *Client) GetStation() *station.StationState {
	return c.station
}

// GetHandler 获取帧处理器
func (c *Client) GetHandler() *FrameHandler {
	return c.handler
}

// SyncDeviceState 同步设备状态
func (c *Client) SyncDeviceState(index uint16, state []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 更新本地站场状态
	dev, ok := c.station.Devices[index]
	if !ok {
		return fmt.Errorf("device %d not found", index)
	}

	switch dev.Type {
	case station.DeviceTurnout:
		if turnState, ok := c.station.Turnouts[dev.Name]; ok {
			// 根据状态字节更新道岔状态
			switch state[0] & 0x07 {
			case 0x01:
				turnState.SetNormal()
			case 0x02:
				turnState.SetReverse()
			default:
				turnState.SetNoIndication()
			}
			turnState.Occupied = (state[0] & 0x10) != 0
			turnState.Locked = (state[0] & 0x20) != 0
		}
	case station.DeviceSignal:
		if sigState, ok := c.station.Signals[dev.Name]; ok {
			sigState.Lights = state[0]
		}
	case station.DeviceSection:
		if secState, ok := c.station.Sections[dev.Name]; ok {
			secState.Occupied = (state[0] & 0x01) != 0
			secState.Locked = (state[0] & 0x02) != 0
		}
	}

	return nil
}

// SyncAllDeviceStates 同步所有设备状态
func (c *Client) SyncAllDeviceStates(states map[uint16][]byte) error {
	for index, state := range states {
		if err := c.SyncDeviceState(index, state); err != nil {
			return err
		}
	}
	return nil
}
