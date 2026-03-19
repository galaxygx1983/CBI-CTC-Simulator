// internal/grpc/server.go
package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"

	pb "ctc-simulator/internal/pb/cbi/v1"
	"ctc-simulator/internal/station"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// Server gRPC服务端
type Server struct {
	pb.UnimplementedCBISimulatorServer

	address  string
	listener net.Listener
	server  *grpc.Server
	stream  *StreamManager
	handler *FrameHandler

	station   *station.StationState
	connected bool

	mu sync.RWMutex
}

// NewServer 创建服务端
func NewServer(address string) *Server {
	s := &Server{
		address: address,
		station: station.NewStationState(),
	}
	s.handler = NewFrameHandler(s)
	return s
}

// Address 返回监听地址
func (s *Server) Address() string {
	return s.address
}

// IsRunning 返回运行状态
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.server != nil
}

// Start 启动服务
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return fmt.Errorf("server already running")
	}

	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	s.listener = lis
	s.server = grpc.NewServer()
	s.stream = NewStreamManager(s)

	pb.RegisterCBISimulatorServer(s.server, s)

	// Capture server before goroutine to avoid race condition
	server := s.server
	go func() {
		log.Infof("gRPC server listening on %s", s.address)
		if err := server.Serve(lis); err != nil {
			log.Errorf("Serve failed: %v", err)
		}
	}()

	return nil
}

// Stop 停止服务
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stream != nil {
		s.stream.Stop()
		log.Info("Stream manager stopped")
	}

	if s.server == nil {
		return nil
	}

	// Stop the gRPC server (this will close the listener)
	s.server.Stop()
	s.server = nil

	// Close the listener explicitly
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
		log.Info("Listener closed")
	}

	log.Info("gRPC server stopped")
	return nil
}

// GetStation 获取站场状态
func (s *Server) GetStation() *station.StationState {
	return s.station
}

// GetStream 获取流管理器
func (s *Server) GetStream() *StreamManager {
	return s.stream
}

// GetHandler 获取帧处理器
func (s *Server) GetHandler() *FrameHandler {
	return s.handler
}

// FrameStream 实现双向流
func (s *Server) FrameStream(stream grpc.BidiStreamingServer[pb.FrameRequest, pb.FrameResponse]) error {
	log.Info("Client connected to FrameStream")

	ctx := stream.Context()

	// 启动流管理
	if err := s.stream.Start(ctx, stream); err != nil {
		return err
	}
	defer func() {
		s.stream.Stop()
		s.mu.Lock()
		s.connected = false
		s.mu.Unlock()
		log.Info("Client disconnected from FrameStream")
	}()

	// 接收循环 - 不使用 select/default，直接调用 Recv()
	// Recv() 会在 context 取消时返回 error (io.EOF 或 codes.Canceled)
	for {
		req, err := stream.Recv()
		if err != nil {
			// context 取消或连接关闭时返回 error
			log.Infof("Recv error (context done or connection closed): %v", err)
			return err
		}

		if req.Frame != nil {
			frame := ProtoToFrame(req.Frame)
			// 直接调用 handler 处理帧
			if err := s.handler.HandleFrame(frame); err != nil {
				log.Warnf("Handle frame failed: %v", err)
			}
		}
	}
}