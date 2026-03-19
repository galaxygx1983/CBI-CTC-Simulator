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

// TCPTransport TCP传输实现
type TCPTransport struct {
	Port     int
	listener net.Listener
	conns    map[string]net.Conn
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	handler  func(data []byte)
	running  bool
}

// NewTCPTransport 创建TCP传输
func NewTCPTransport(port int) *TCPTransport {
	return &TCPTransport{
		Port:  port,
		conns: make(map[string]net.Conn),
	}
}

// Type 返回传输类型
func (t *TCPTransport) Type() string {
	return string(TransportTCP)
}

// Start 启动TCP服务
func (t *TCPTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return fmt.Errorf("transport already running")
	}

	addr := fmt.Sprintf(":%d", t.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	t.listener = listener
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.running = true

	// 获取实际端口
	t.Port = listener.Addr().(*net.TCPAddr).Port

	log.Infof("TCP transport listening on :%d", t.Port)

	go t.acceptLoop()

	return nil
}

// acceptLoop 接收连接循环
func (t *TCPTransport) acceptLoop() {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			conn, err := t.listener.Accept()
			if err != nil {
				if t.ctx.Err() != nil {
					return // 正常关闭
				}
				log.Errorf("Accept error: %v", err)
				continue
			}

			connID := conn.RemoteAddr().String()
			log.Infof("New connection from %s", connID)

			t.mu.Lock()
			t.conns[connID] = conn
			t.mu.Unlock()

			go t.readLoop(connID, conn)
		}
	}
}

// readLoop 读取数据循环
func (t *TCPTransport) readLoop(connID string, conn net.Conn) {
	defer func() {
		conn.Close()
		t.mu.Lock()
		delete(t.conns, connID)
		t.mu.Unlock()
		log.Infof("Connection %s closed", connID)
	}()

	buf := make([]byte, 1024)
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err := conn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if t.ctx.Err() != nil {
					return // 正常关闭
				}
				log.Debugf("Read error from %s: %v", connID, err)
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

// Stop 停止TCP服务
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

	for _, conn := range t.conns {
		conn.Close()
	}
	t.conns = make(map[string]net.Conn)

	if t.listener != nil {
		t.listener.Close()
	}

	log.Info("TCP transport stopped")
	return nil
}

// Send 发送数据到所有连接
func (t *TCPTransport) Send(data []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.conns) == 0 {
		return fmt.Errorf("no active connections")
	}

	for id, conn := range t.conns {
		_, err := conn.Write(data)
		if err != nil {
			log.Errorf("Write to %s failed: %v", id, err)
			continue
		}
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

// IsConnected 返回是否有活跃连接
func (t *TCPTransport) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.conns) > 0
}

// IsListening 返回是否在监听
func (t *TCPTransport) IsListening() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

// GetPort 获取实际监听端口
func (t *TCPTransport) GetPort() int {
	return t.Port
}