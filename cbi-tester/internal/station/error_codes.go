// internal/station/error_codes.go
package station

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ErrorCode 错误码定义
type ErrorCode struct {
	Code    int    // 错误码
	Message string // 中文描述
}

// ErrorCodeTable 错误码表
type ErrorCodeTable struct {
	Codes map[int]*ErrorCode // 按错误码索引
}

// LoadErrorCodeTable 从 Error.sys 文件加载错误码表（支持 GB2312 编码）
func LoadErrorCodeTable(filename string) (*ErrorCodeTable, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// 使用 GB2312 解码器读取文件
	decoder := transform.NewReader(file, simplifiedchinese.GB18030.NewDecoder())

	table := &ErrorCodeTable{
		Codes: make(map[int]*ErrorCode),
	}

	scanner := bufio.NewScanner(decoder)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行、注释和标题行
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "错误码") {
			continue
		}

		// 解析行：错误码\t 中文描述
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			// 尝试用空格分割
			parts = strings.Fields(line)
			if len(parts) < 2 {
				continue // 忽略格式错误的行
			}
		}

		code, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue // 忽略无法解析的行
		}

		message := strings.TrimSpace(parts[1])

		table.Codes[code] = &ErrorCode{
			Code:    code,
			Message: message,
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}

	return table, nil
}

// GetError 按错误码获取错误描述
func (t *ErrorCodeTable) GetError(code int) (*ErrorCode, bool) {
	errCode, ok := t.Codes[code]
	return errCode, ok
}

// GetAllErrors 获取所有错误码
func (t *ErrorCodeTable) GetAllErrors() []*ErrorCode {
	result := make([]*ErrorCode, 0, len(t.Codes))
	for _, code := range t.Codes {
		result = append(result, code)
	}
	return result
}

// GetErrorCount 获取错误码数量
func (t *ErrorCodeTable) GetErrorCount() int {
	return len(t.Codes)
}
