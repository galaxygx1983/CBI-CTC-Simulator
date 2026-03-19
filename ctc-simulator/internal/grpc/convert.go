// internal/grpc/convert.go
package grpc

import (
	pb "ctc-simulator/internal/pb/cbi/v1"
	"ctc-simulator/internal/protocol"
)

// frameTypeToProto 协议帧类型转 protobuf
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

// protoFrameTypeToProtocol protobuf 帧类型转协议
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
		return protocol.FrameType(0xFF)
	}
}

// FrameToProto 将 Frame 转换为 Proto 格式
func FrameToProto(frame *protocol.Frame) *pb.Frame {
	pbType := frameTypeToProto(frame.Type)

	pbFrame := &pb.Frame{
		Header:    []byte{frame.Header},
		HeaderLen: uint32(frame.HeaderLen),
		Version:   uint32(frame.Version),
		SendSeq:   uint32(frame.SendSeq),
		AckSeq:    uint32(frame.AckSeq),
		Type:      pbType,
		Crc:       uint32(frame.CRC),
	}

	if frame.DataLength > 0 {
		dataLen := uint32(frame.DataLength)
		pbFrame.DataLength = &dataLen
		pbFrame.Data = frame.Data
	}

	return pbFrame
}

// ProtoToFrame 将 Proto 格式转换为 Frame
func ProtoToFrame(pbFrame *pb.Frame) *protocol.Frame {
	frame := &protocol.Frame{
		Header:    byte(pbFrame.Header[0]),
		HeaderLen: byte(pbFrame.HeaderLen),
		Version:   byte(pbFrame.Version),
		SendSeq:   byte(pbFrame.SendSeq),
		AckSeq:    byte(pbFrame.AckSeq),
		Type:      protoFrameTypeToProtocol(pbFrame.Type),
		CRC:       uint16(pbFrame.Crc),
	}

	if pbFrame.DataLength != nil {
		frame.DataLength = uint16(*pbFrame.DataLength)
		frame.Data = pbFrame.Data
	}

	return frame
}
