// internal/scenario/engine_test.go
package scenario

import (
	"context"
	"testing"
	"time"
)

func TestNewEngine(t *testing.T) {
	script := &Script{Name: "test"}
	engine := NewEngine(script)

	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}

	if engine.Name() != "test" {
		t.Errorf("Expected name 'test', got %s", engine.Name())
	}
}

func TestEngineStartStop(t *testing.T) {
	script := &Script{
		Name: "test",
		Actions: []Action{
			{DelayMs: 100, Type: ActionTypeWait},
		},
	}

	engine := NewEngine(script)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !engine.IsRunning() {
		t.Error("Engine should be running")
	}

	engine.Stop()

	if engine.IsRunning() {
		t.Error("Engine should not be running")
	}
}