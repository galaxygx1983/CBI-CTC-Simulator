// internal/protocol/codec.go
package protocol

// CalculateCRC 计算XMODEM CRC16校验和
// XMODEM CRC-16-CCITT: 多项式 0x1021, 初始值 0x0000
func CalculateCRC(data []byte) uint16 {
	crc := uint16(0x0000)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// EscapeData 数据转义（发送前）
// 0x7D -> 0x7F 0xFD
// 0x7E -> 0x7F 0xFE
// 0x7F -> 0x7F 0xFF
func EscapeData(data []byte) []byte {
	result := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case 0x7D:
			result = append(result, 0x7F, 0xFD)
		case 0x7E:
			result = append(result, 0x7F, 0xFE)
		case 0x7F:
			result = append(result, 0x7F, 0xFF)
		default:
			result = append(result, b)
		}
	}
	return result
}

// UnescapeData 数据反转义（接收后）
func UnescapeData(data []byte) []byte {
	result := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		if data[i] == 0x7F && i+1 < len(data) {
			switch data[i+1] {
			case 0xFD:
				result = append(result, 0x7D)
				i += 2
			case 0xFE:
				result = append(result, 0x7E)
				i += 2
			case 0xFF:
				result = append(result, 0x7F)
				i += 2
			default:
				result = append(result, data[i])
				i++
			}
		} else {
			result = append(result, data[i])
			i++
		}
	}
	return result
}