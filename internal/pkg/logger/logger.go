// Package logger provides structured logging for KubeVirt Shepherd.
//
// Uses zap with AtomicLevel for hot-reload support.
// JSON format for production, console for development.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/pkg/logger
package logger

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// global is the package-level logger instance.
	global      *zap.Logger
	atomicLevel zap.AtomicLevel
	once        sync.Once
)

// Init initializes the global logger.
// level: debug, info, warn, error
// format: json or console
func Init(level, format string) error {
	var initErr error
	once.Do(func() {
		atomicLevel = zap.NewAtomicLevel()
		if err := atomicLevel.UnmarshalText([]byte(level)); err != nil {
			initErr = fmt.Errorf("parse log level %q: %w", level, err)
			return
		}

		var cfg zap.Config
		switch format {
		case "console":
			cfg = zap.NewDevelopmentConfig()
			cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		default:
			cfg = zap.NewProductionConfig()
		}
		cfg.Level = atomicLevel

		logger, err := cfg.Build(zap.AddCallerSkip(1))
		if err != nil {
			initErr = fmt.Errorf("build logger: %w", err)
			return
		}
		global = logger
	})
	return initErr
}

// SetLevel dynamically changes the log level (hot-reload support).
func SetLevel(level string) error {
	return atomicLevel.UnmarshalText([]byte(level))
}

// GetLevel returns the current log level.
func GetLevel() zapcore.Level {
	return atomicLevel.Level()
}

// L returns the global logger. Panics if Init has not been called.
func L() *zap.Logger {
	if global == nil {
		panic("logger.Init() must be called before logger.L()")
	}
	return global
}

// S returns the global sugared logger.
func S() *zap.SugaredLogger {
	return L().Sugar()
}

// Debug logs a message at DebugLevel.
func Debug(msg string, fields ...zap.Field) {
	L().Debug(msg, fields...)
}

// Info logs a message at InfoLevel.
func Info(msg string, fields ...zap.Field) {
	L().Info(msg, fields...)
}

// Warn logs a message at WarnLevel.
func Warn(msg string, fields ...zap.Field) {
	L().Warn(msg, fields...)
}

// Error logs a message at ErrorLevel.
func Error(msg string, fields ...zap.Field) {
	L().Error(msg, fields...)
}

// Fatal logs a message at FatalLevel then calls os.Exit(1).
func Fatal(msg string, fields ...zap.Field) {
	L().Fatal(msg, fields...)
}

// With creates a child logger with additional fields.
func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
}

// HTTPHandler returns an http.Handler that allows dynamic log level changes.
// Mount at /log/level for runtime hot-reload (zap AtomicLevel best practice).
//
// Usage:
//
//	GET  /log/level          → returns current level
//	PUT  /log/level -d '{"level":"debug"}' → changes level
func HTTPHandler() *zap.AtomicLevel {
	return &atomicLevel
}

// Sync flushes any buffered log entries.
func Sync() error {
	if global == nil {
		return nil
	}
	return global.Sync()
}
