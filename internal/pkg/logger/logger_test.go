package logger

import (
	"sync"
	"testing"

	"go.uber.org/zap/zapcore"
)

func resetLogger() {
	global = nil
	once = sync.Once{}
}

// TestInit uses table-driven tests (Go best practice from go.dev/doc).
func TestInit(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		format    string
		wantLevel zapcore.Level
		wantErr   bool
	}{
		{"json info", "info", "json", zapcore.InfoLevel, false},
		{"console debug", "debug", "console", zapcore.DebugLevel, false},
		{"json warn", "warn", "json", zapcore.WarnLevel, false},
		{"json error", "error", "json", zapcore.ErrorLevel, false},
		{"invalid level", "invalid", "json", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLogger()
			err := Init(tt.level, tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init(%q, %q) error = %v, wantErr %v", tt.level, tt.format, err, tt.wantErr)
				return
			}
			if !tt.wantErr && GetLevel() != tt.wantLevel {
				t.Errorf("GetLevel() = %v, want %v", GetLevel(), tt.wantLevel)
			}
		})
	}
}

// TestSetLevel verifies dynamic log level changes via AtomicLevel.
func TestSetLevel(t *testing.T) {
	resetLogger()

	if err := Init("info", "json"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	tests := []struct {
		name      string
		level     string
		wantLevel zapcore.Level
		wantErr   bool
	}{
		{"to debug", "debug", zapcore.DebugLevel, false},
		{"to error", "error", zapcore.ErrorLevel, false},
		{"back to info", "info", zapcore.InfoLevel, false},
		{"invalid", "bogus", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetLevel(%q) error = %v, wantErr %v", tt.level, err, tt.wantErr)
				return
			}
			if !tt.wantErr && GetLevel() != tt.wantLevel {
				t.Errorf("GetLevel() = %v, want %v", GetLevel(), tt.wantLevel)
			}
		})
	}
}

func TestL_PanicsWithoutInit(t *testing.T) {
	resetLogger()

	defer func() {
		if r := recover(); r == nil {
			t.Error("L() should panic without Init()")
		}
	}()

	L()
}

func TestLoggingFunctions(t *testing.T) {
	resetLogger()

	if err := Init("debug", "json"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// These should not panic
	Debug("test debug")
	Info("test info")
	Warn("test warn")
	Error("test error")
}

func TestWith(t *testing.T) {
	resetLogger()

	if err := Init("info", "json"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	child := With()
	if child == nil {
		t.Error("With() returned nil")
	}
}

func TestS(t *testing.T) {
	resetLogger()

	if err := Init("info", "json"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	sugar := S()
	if sugar == nil {
		t.Error("S() returned nil")
	}
}

func TestHTTPHandler(t *testing.T) {
	resetLogger()

	if err := Init("info", "json"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	handler := HTTPHandler()
	if handler == nil {
		t.Error("HTTPHandler() returned nil")
	}
	if handler.Level() != zapcore.InfoLevel {
		t.Errorf("HTTPHandler().Level() = %v, want InfoLevel", handler.Level())
	}
}

func TestSync(t *testing.T) {
	resetLogger()

	// Sync on nil logger should not error
	if err := Sync(); err != nil {
		t.Errorf("Sync() on nil logger error = %v", err)
	}

	if err := Init("info", "json"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Sync may return error on stderr (expected in test), just ensure no panic
	_ = Sync()
}
