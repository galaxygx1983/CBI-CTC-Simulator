// internal/grpc/convert.go
package grpc

import (
	pb "cbi-simulator/internal/pb/cbi/v1"
	"cbi-simulator/internal/protocol"
)

// frameToProtoFrame 将协议帧转换为protobuf帧
func frameToProtoFrame(frame *protocol.Frame) *pb.Frame {
	protoFrame := &pb.Frame{
		Header:    []byte{frame.Header},
		HeaderLen: uint32(frame.HeaderLen),
		Version:   uint32(frame.Version),
		SendSeq:   uint32(frame.SendSeq),
		AckSeq:    uint32(frame.AckSeq),
		Type:      frameTypeToProto(frame.Type),
		Crc:       uint32(frame.CRC),
	}

	// 仅对数据帧设置数据字段
	if !frame.Type.IsControlFrame() && len(frame.Data) > 0 {
		dataLen := uint32(frame.DataLength)
		protoFrame.DataLength = &dataLen
		protoFrame.Data = frame.Data
	}

	return protoFrame
}

// protoFrameToFrame 将protobuf帧转换为协议帧
func protoFrameToFrame(protoFrame *pb.Frame) *protocol.Frame {
	frame := &protocol.Frame{
		Header:    protocol.FrameHeader,
		HeaderLen: byte(protoFrame.HeaderLen),
		Version:   byte(protoFrame.Version),
		SendSeq:   byte(protoFrame.SendSeq),
		AckSeq:    byte(protoFrame.AckSeq),
		Type:      protoFrameTypeToProtocol(protoFrame.Type),
		CRC:       uint16(protoFrame.Crc),
	}

	// 处理可选数据字段
	if protoFrame.DataLength != nil {
		frame.DataLength = uint16(*protoFrame.DataLength)
	}
	if len(protoFrame.Data) > 0 {
		frame.Data = protoFrame.Data
	}

	return frame
}

// frameTypeToProto 协议帧类型转protobuf
func frameTypeToProto(ft protocol.FrameType) pb.FrameType {
	switch ft {
	case protocol.DC2:
		return pb.FrameType_FRAME_TYPE_DC2
	case protocol.DC3:
		return pb.FrameType_FRAME_TYPE_DC3
	case protocol.ACK:
		return pb.FrameType_FRAME_TYPE_ACK
	case protocol.NACK:
		return pb.FrameType_FRAME_TYPE_NACK
	case protocol.VERROR:
		return pb.FrameType_FRAME_TYPE_VERROR
	case protocol.SDCI:
		return pb.FrameType_FRAME_TYPE_SDCI
	case protocol.SDI:
		return pb.FrameType_FRAME_TYPE_SDI
	case protocol.SDIQ:
		return pb.FrameType_FRAME_TYPE_SDIQ
	case protocol.FIR:
		return pb.FrameType_FRAME_TYPE_FIR
	case protocol.RSR:
		return pb.FrameType_FRAME_TYPE_RSR
	case protocol.BCC:
		return pb.FrameType_FRAME_TYPE_BCC
	case protocol.ACQ:
		return pb.FrameType_FRAME_TYPE_ACQ
	case protocol.ACA:
		return pb.FrameType_FRAME_TYPE_ACA
	case protocol.TSQ:
		return pb.FrameType_FRAME_TYPE_TSQ
	case protocol.TSD:
		return pb.FrameType_FRAME_TYPE_TSD
	default:
		return pb.FrameType_FRAME_TYPE_UNSPECIFIED
	}
}

// protoFrameTypeToProtocol protobuf帧类型转协议
func protoFrameTypeToProtocol(pft pb.FrameType) protocol.FrameType {
	switch pft {
	case pb.FrameType_FRAME_TYPE_DC2:
		return protocol.DC2
	case pb.FrameType_FRAME_TYPE_DC3:
		return protocol.DC3
	case pb.FrameType_FRAME_TYPE_ACK:
		return protocol.ACK
	case pb.FrameType_FRAME_TYPE_NACK:
		return protocol.NACK
	case pb.FrameType_FRAME_TYPE_VERROR:
		return protocol.VERROR
	case pb.FrameType_FRAME_TYPE_SDCI:
		return protocol.SDCI
	case pb.FrameType_FRAME_TYPE_SDI:
		return protocol.SDI
	case pb.FrameType_FRAME_TYPE_SDIQ:
		return protocol.SDIQ
	case pb.FrameType_FRAME_TYPE_FIR:
		return protocol.FIR
	case pb.FrameType_FRAME_TYPE_RSR:
		return protocol.RSR
	case pb.FrameType_FRAME_TYPE_BCC:
		return protocol.BCC
	case pb.FrameType_FRAME_TYPE_ACQ:
		return protocol.ACQ
	case pb.FrameType_FRAME_TYPE_ACA:
		return protocol.ACA
	case pb.FrameType_FRAME_TYPE_TSQ:
		return protocol.TSQ
	case pb.FrameType_FRAME_TYPE_TSD:
		return protocol.TSD
	default:
		return protocol.FrameType(0xFF) // Unknown
	}
}