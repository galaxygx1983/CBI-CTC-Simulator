// internal/station/error_codes_test.go
package station

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestLoadErrorCodeTable(t *testing.T) {
	// 创建测试文件（使用GBK编码）
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Error.sys")

	content := `错误码	中文描述
0	错误办理
1	运行表满
2	区段占用
3	敌对进路
4	不能转岔
5	灯丝断丝
6	道岔锁闭
`

	// 将内容转换为GBK编码
	encoder := simplifiedchinese.GB18030.NewEncoder()
	encodedContent, err := encoder.Bytes([]byte(content))
	if err != nil {
		t.Fatalf("Encode content failed: %v", err)
	}

	if err := os.WriteFile(testFile, encodedContent, 0644); err != nil {
		t.Fatalf("Write test file failed: %v", err)
	}

	// 解析
	table, err := LoadErrorCodeTable(testFile)
	if err != nil {
		t.Fatalf("LoadErrorCodeTable failed: %v", err)
	}

	// 验证
	if len(table.Codes) != 7 {
		t.Errorf("Expected 7 error codes, got %d", len(table.Codes))
	}

	// 验证查找
	errCode, ok := table.GetError(3)
	if !ok {
		t.Fatal("Error code 3 not found")
	}
	if errCode.Message != "敌对进路" {
		t.Errorf("Expected message '敌对进路', got '%s'", errCode.Message)
	}

	// 验证不存在的错误码
	_, ok = table.GetError(999)
	if ok {
		t.Error("Error code 999 should not exist")
	}
}

func TestGetAllErrors(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Error.sys")

	content := `错误码	中文描述
0	错误办理
1	运行表满
2	区段占用
`

	// 将内容转换为GBK编码
	encoder := simplifiedchinese.GB18030.NewEncoder()
	encodedContent, err := encoder.Bytes([]byte(content))
	if err != nil {
		t.Fatalf("Encode content failed: %v", err)
	}

	if err := os.WriteFile(testFile, encodedContent, 0644); err != nil {
		t.Fatalf("Write test file failed: %v", err)
	}

	table, err := LoadErrorCodeTable(testFile)
	if err != nil {
		t.Fatalf("LoadErrorCodeTable failed: %v", err)
	}

	allErrors := table.GetAllErrors()
	if len(allErrors) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(allErrors))
	}
}
