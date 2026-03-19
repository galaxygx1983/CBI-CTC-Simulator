// internal/transport/tcp.go
package transport

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Type 传输类型
type Type string

const (
	TransportTCP Type = "tcp"
)

// Transport 传输接口
type Transport interface {
	Type() string
	Start(ctx context.Context) error
	Stop() error
	Send(data []byte) error
	SetOnDataHandler(handler func(data []byte))
	IsConnected() bool
}

// TCPTransport TCP 传输实现（客户端模式）
type TCPTransport struct {
	Host     string
	Port     int
	conn     net.Conn
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	handler  func(data []byte)
	running  bool
}

// NewTCPTransport 创建 TCP 传输（客户端）
func NewTCPTransport(host string, port int) *TCPTransport {
	return &TCPTransport{
		Host: host,
		Port: port,
	}
}

// Type 返回传输类型
func (t *TCPTransport) Type() string {
	return string(TransportTCP)
}

// Start 启动 TCP 客户端连接
func (t *TCPTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return fmt.Errorf("transport already running")
	}

	addr := fmt.Sprintf("%s:%d", t.Host, t.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	t.conn = conn
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.running = true

	log.Infof("TCP client connected to %s", addr)

	go t.readLoop()

	return nil
}

// readLoop 读取数据循环
func (t *TCPTransport) readLoop() {
	defer func() {
		if t.conn != nil {
			t.conn.Close()
		}
		log.Info("TCP connection closed")
	}()

	buf := make([]byte, 1024)
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			t.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err := t.conn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if t.ctx.Err() != nil {
					return // 正常关闭
				}
				log.Debugf("Read error: %v", err)
				return
			}

			if n > 0 && t.handler != nil {
				data := make([]byte, n)
				copy(data, buf[:n])
				t.handler(data)
			}
		}
	}
}

// Stop 停止 TCP 客户端
func (t *TCPTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	t.running = false

	if t.cancel != nil {
		t.cancel()
	}

	if t.conn != nil {
		t.conn.Close()
	}

	log.Info("TCP transport stopped")
	return nil
}

// Send 发送数据
func (t *TCPTransport) Send(data []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.conn == nil {
		return fmt.Errorf("no active connection")
	}

	_, err := t.conn.Write(data)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

// Receive 接收数据（阻塞）
func (t *TCPTransport) Receive() ([]byte, error) {
	// 使用回调模式，此方法不实现
	return nil, fmt.Errorf("use SetOnDataHandler instead")
}

// SetOnDataHandler 设置数据接收回调
func (t *TCPTransport) SetOnDataHandler(handler func(data []byte)) {
	t.handler = handler
}

// IsConnected 返回是否已连接
func (t *TCPTransport) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.conn != nil
}
