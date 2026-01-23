package logger

import (
	"log/slog"
	"os"
	"strings"
)

var log *slog.Logger

// Init initializes the global logger with the appropriate level.
// If verbose is true or LOG_LEVEL env var is "debug", debug logging is enabled.
func Init(verbose bool) {
	level := slog.LevelInfo

	// Check for verbose flag or LOG_LEVEL environment variable
	if verbose || strings.EqualFold(os.Getenv("LOG_LEVEL"), "debug") {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stderr, opts)
	log = slog.New(handler)
	slog.SetDefault(log)
}

// Debug logs a debug message with optional key-value pairs.
func Debug(msg string, args ...any) {
	log.Debug(msg, args...)
}

// Info logs an info message with optional key-value pairs.
func Info(msg string, args ...any) {
	log.Info(msg, args...)
}

// Warn logs a warning message with optional key-value pairs.
func Warn(msg string, args ...any) {
	log.Warn(msg, args...)
}

// Error logs an error message with optional key-value pairs.
func Error(msg string, args ...any) {
	log.Error(msg, args...)
}
