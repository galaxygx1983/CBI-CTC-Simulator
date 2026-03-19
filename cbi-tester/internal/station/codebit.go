// internal/station/codebit.go
package station

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// CodebitTable 码位表
type CodebitTable struct {
	Objects map[uint16]*Device // 按索引索引
	ByName  map[string]*Device // 按名称索引
	Devices []*Device          // 设备列表
}

// LoadCodebitTable 从文件加载码位表（支持 GB2312 编码）
func LoadCodebitTable(filename string) (*CodebitTable, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// 使用 GB2312 解码器读取文件
	decoder := transform.NewReader(file, simplifiedchinese.GB18030.NewDecoder())

	table := &CodebitTable{
		Objects: make(map[uint16]*Device),
		ByName:  make(map[string]*Device),
	}

	scanner := bufio.NewScanner(decoder)
	section := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// 识别节
		if line == "[objects]" {
			section = "objects"
			continue
		}
		if line == "[zlobjects]" {
			section = "zlobjects"
			continue
		}

		// 解析行
		if strings.HasPrefix(line, "#,") {
			switch section {
			case "objects":
				dev, err := parseObjectsLine(line)
				if err != nil {
					continue // 忽略错误行
				}
				dev.Type = DetectDeviceType(dev.Name)
				table.Objects[dev.Index] = dev
				table.ByName[dev.Name] = dev
				table.Devices = append(table.Devices, dev)

			case "zlobjects":
				dev, err := parseZlobjectsLine(line)
				if err != nil {
					continue // 忽略错误行
				}
				// 合并到已有设备
				if existing, ok := table.ByName[dev.Name]; ok {
					existing.ByteOffset = dev.ByteOffset
					existing.BitOffset = dev.BitOffset
				} else {
					dev.Type = DetectDeviceType(dev.Name)
					table.ByName[dev.Name] = dev
					table.Devices = append(table.Devices, dev)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}

	return table, nil
}

// parseObjectsLine 解析objects行: #,设备名,索引号
func parseObjectsLine(line string) (*Device, error) {
	// 格式: #,设备名,索引号
	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return nil, errors.New("invalid format")
	}

	name := strings.TrimSpace(parts[1])
	index, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}

	return &Device{
		Name:  name,
		Index: uint16(index),
	}, nil
}

// parseZlobjectsLine 解析zlobjects行: #,设备名,字节偏移,位偏移
func parseZlobjectsLine(line string) (*Device, error) {
	// 格式: #,设备名,字节偏移,位偏移
	parts := strings.Split(line, ",")
	if len(parts) < 4 {
		return nil, errors.New("invalid format")
	}

	name := strings.TrimSpace(parts[1])
	byteOffset, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return nil, fmt.Errorf("parse byte offset: %w", err)
	}
	bitOffset, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err != nil {
		return nil, fmt.Errorf("parse bit offset: %w", err)
	}

	return &Device{
		Name:       name,
		ByteOffset: uint16(byteOffset),
		BitOffset:  uint8(bitOffset),
	}, nil
}

// DetectDeviceType 检测设备类型
func DetectDeviceType(name string) DeviceType {
	// D开头+数字 = 信号机
	if matched, _ := regexp.MatchString(`^D\d+`, name); matched {
		return DeviceSignal
	}

	// 纯数字 = 道岔
	if matched, _ := regexp.MatchString(`^\d+$`, name); matched {
		return DeviceTurnout
	}

	// 其他 = 区段
	return DeviceSection
}

// GetDeviceByIndex 按索引获取设备
func (t *CodebitTable) GetDeviceByIndex(index uint16) (*Device, bool) {
	dev, ok := t.Objects[index]
	return dev, ok
}

// GetDeviceByName 按名称获取设备
func (t *CodebitTable) GetDeviceByName(name string) (*Device, bool) {
	dev, ok := t.ByName[name]
	return dev, ok
}

// GetDevicesByType 按类型获取设备列表
func (t *CodebitTable) GetDevicesByType(deviceType DeviceType) []*Device {
	var result []*Device
	for _, dev := range t.Devices {
		if dev.Type == deviceType {
			result = append(result, dev)
		}
	}
	return result
}
