// internal/scenario/engine.go
package scenario

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// SendFunc 发送帧的函数类型
type SendFunc func(action Action) error

// Engine 场景执行引擎
type Engine struct {
	script   *Script
	name     string
	running  bool
	paused   bool
	sendFunc SendFunc

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex
}

// NewEngine 创建场景引擎
func NewEngine(script *Script) *Engine {
	return &Engine{
		script: script,
		name:   script.Name,
	}
}

// Name 返回场景名称
func (e *Engine) Name() string {
	return e.name
}

// SetSendFunc 设置发送函数
func (e *Engine) SetSendFunc(fn SendFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sendFunc = fn
}

// Start 启动场景执行
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("engine already running")
	}

	e.ctx, e.cancel = context.WithCancel(ctx)
	e.running = true
	e.paused = false
	e.mu.Unlock()

	go e.run()

	log.Infof("Scenario engine started: %s", e.name)
	return nil
}

// Stop 停止执行
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	e.mu.Unlock()

	if e.cancel != nil {
		e.cancel()
	}

	log.Infof("Scenario engine stopped: %s", e.name)
}

// Pause 暂停执行
func (e *Engine) Pause() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("engine not running")
	}

	e.paused = true
	log.Infof("Scenario engine paused: %s", e.name)
	return nil
}

// Resume 恢复执行
func (e *Engine) Resume() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("engine not running")
	}

	e.paused = false
	log.Infof("Scenario engine resumed: %s", e.name)
	return nil
}

// IsRunning 返回运行状态
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// IsPaused 返回暂停状态
func (e *Engine) IsPaused() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.paused
}

// run 执行循环
func (e *Engine) run() {
	for _, action := range e.script.Actions {
		// 检查是否停止
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		// 等待延迟
		if action.DelayMs > 0 {
			select {
			case <-time.After(time.Duration(action.DelayMs) * time.Millisecond):
			case <-e.ctx.Done():
				return
			}
		}

		// 检查暂停
		e.waitForResume()

		// 执行动作
		e.executeAction(action)
	}

	// 所有动作执行完成
	e.Stop()
}

// waitForResume 等待恢复
func (e *Engine) waitForResume() {
	for {
		e.mu.RLock()
		paused := e.paused
		e.mu.RUnlock()

		if !paused {
			return
		}

		select {
		case <-time.After(100 * time.Millisecond):
		case <-e.ctx.Done():
			return
		}
	}
}

// executeAction 执行单个动作
func (e *Engine) executeAction(action Action) {
	e.mu.RLock()
	sendFunc := e.sendFunc
	e.mu.RUnlock()

	if sendFunc == nil {
		log.Warnf("No send function set, skipping action: %s", action.Type)
		return
	}

	log.Infof("Executing action: %s (device=%d, command=%s)", action.Type, action.DeviceIndex, action.Command)

	if err := sendFunc(action); err != nil {
		log.Errorf("Execute action failed: %v", err)
	}
}