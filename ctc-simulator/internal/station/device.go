// internal/station/device.go
package station

import "fmt"

// DeviceType 设备类型
type DeviceType int

const (
	DeviceTurnout DeviceType = iota // 道岔
	DeviceSignal                     // 信号机
	DeviceSection                    // 区段
)

// String 返回设备类型名称
func (dt DeviceType) String() string {
	names := map[DeviceType]string{
		DeviceTurnout: "turnout",
		DeviceSignal:  "signal",
		DeviceSection: "section",
	}
	return names[dt]
}

// Device 设备信息
type Device struct {
	Index      uint16     // 设备索引
	Name       string     // 设备名称
	Type       DeviceType // 设备类型
	ByteOffset uint16     // 字节偏移
	BitOffset  uint8      // 位偏移
}

// TurnoutState 道岔状态
type TurnoutState struct {
	Position      uint8 // 0=未知, 1=定位, 2=反位, 3=四开
	Occupied      bool  // 区段占用
	Locked        bool  // 道岔锁闭
	SectionLocked bool  // 区段锁闭
}

// NewTurnoutState 创建道岔状态
func NewTurnoutState() *TurnoutState {
	return &TurnoutState{Position: 3} // 初始为四开
}

func (s *TurnoutState) SetNormal()       { s.Position = 1 }
func (s *TurnoutState) SetReverse()      { s.Position = 2 }
func (s *TurnoutState) SetNoIndication() { s.Position = 3 }
func (s *TurnoutState) IsNormal() bool   { return s.Position == 1 }
func (s *TurnoutState) IsReverse() bool  { return s.Position == 2 }
func (s *TurnoutState) IsNoIndication() bool { return s.Position == 3 }

// ToByte 转换为状态字节
func (s *TurnoutState) ToByte() byte {
	var b byte
	switch s.Position {
	case 1:
		b = 0x01 // 定位
	case 2:
		b = 0x02 // 反位
	default:
		b = 0x04 // 四开/无表示
	}
	if s.Occupied {
		b |= 0x10
	}
	if s.SectionLocked {
		b |= 0x08
	}
	if s.Locked {
		b |= 0x20
	}
	return b
}

// SignalState 信号机状态
type SignalState struct {
	Lights byte // 灯位组合
}

// 信号灯位定义
const (
	SignalOff    = 0x00
	SignalBlue   = 0x02
	SignalWhite  = 0x04
	SignalRed    = 0x08
	SignalGreen  = 0x10
	SignalYellow = 0x20
)

// NewSignalState 创建信号机状态
func NewSignalState() *SignalState {
	return &SignalState{Lights: SignalBlue} // 初始为蓝灯
}

func (s *SignalState) SetOff()    { s.Lights = SignalOff }
func (s *SignalState) SetBlue()   { s.Lights = SignalBlue }
func (s *SignalState) SetWhite()  { s.Lights = SignalWhite }
func (s *SignalState) SetRed()    { s.Lights = SignalRed }
func (s *SignalState) SetGreen()  { s.Lights = SignalGreen }
func (s *SignalState) SetYellow() { s.Lights = SignalYellow }

func (s *SignalState) IsOff() bool   { return s.Lights == SignalOff }
func (s *SignalState) IsBlue() bool  { return s.Lights == SignalBlue }
func (s *SignalState) IsWhite() bool { return s.Lights == SignalWhite }
func (s *SignalState) IsRed() bool    { return s.Lights == SignalRed }
func (s *SignalState) IsGreen() bool { return s.Lights == SignalGreen }
func (s *SignalState) IsYellow() bool { return s.Lights == SignalYellow }

// ToByte 转换为状态字节
func (s *SignalState) ToByte() byte {
	return s.Lights
}

// SectionState 区段状态
type SectionState struct {
	Occupied bool
	Locked   bool
}

// NewSectionState 创建区段状态
func NewSectionState() *SectionState {
	return &SectionState{}
}

func (s *SectionState) SetOccupied() { s.Occupied = true }
func (s *SectionState) SetClear()    { s.Occupied = false }
func (s *SectionState) SetLocked()   { s.Locked = true }
func (s *SectionState) SetUnlocked() { s.Locked = false }
func (s *SectionState) IsOccupied() bool { return s.Occupied }
func (s *SectionState) IsLocked() bool   { return s.Locked }

// ToByte 转换为状态字节
func (s *SectionState) ToByte(offset uint8) byte {
	var b byte
	if s.Occupied {
		b |= 0x01
	}
	if s.Locked {
		b |= 0x02
	}
	if offset == 4 {
		b <<= 4
	}
	return b
}

// StationState 站场状态
type StationState struct {
	Turnouts map[string]*TurnoutState
	Signals  map[string]*SignalState
	Sections map[string]*SectionState
	Devices  map[uint16]*Device // 按索引索引的设备
}

// NewStationState 创建站场状态
func NewStationState() *StationState {
	return &StationState{
		Turnouts: make(map[string]*TurnoutState),
		Signals:  make(map[string]*SignalState),
		Sections: make(map[string]*SectionState),
		Devices:  make(map[uint16]*Device),
	}
}

// AddDevice 添加设备
func (s *StationState) AddDevice(dev *Device) {
	s.Devices[dev.Index] = dev
	switch dev.Type {
	case DeviceTurnout:
		s.Turnouts[dev.Name] = NewTurnoutState()
	case DeviceSignal:
		s.Signals[dev.Name] = NewSignalState()
	case DeviceSection:
		s.Sections[dev.Name] = NewSectionState()
	}
}

// GetDeviceState 获取设备状态字节
func (s *StationState) GetDeviceState(index uint16) (byte, error) {
	dev, ok := s.Devices[index]
	if !ok {
		return 0, fmt.Errorf("device index %d not found", index)
	}

	switch dev.Type {
	case DeviceTurnout:
		if state, ok := s.Turnouts[dev.Name]; ok {
			return state.ToByte(), nil
		}
	case DeviceSignal:
		if state, ok := s.Signals[dev.Name]; ok {
			return state.ToByte(), nil
		}
	case DeviceSection:
		if state, ok := s.Sections[dev.Name]; ok {
			return state.ToByte(dev.BitOffset), nil
		}
	}

	return 0, fmt.Errorf("state not found for device %s", dev.Name)
}