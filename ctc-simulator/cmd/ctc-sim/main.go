// cmd/ctc-sim/main.go
package main

import (
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

func main() {
	// 配置日志输出到文件
	logDir := "./logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Warnf("Failed to create log dir: %v", err)
	}

	logFile := filepath.Join(logDir, "ctc-"+time.Now().Format("20060102-150405")+".log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Warnf("Failed to open log file: %v", err)
	} else {
		defer file.Close()
		log.SetOutput(file)
		log.SetLevel(log.DebugLevel)
		log.Infof("Logging to file: %s", logFile)
	}

	if err := Execute(); err != nil {
		os.Exit(1)
	}
}