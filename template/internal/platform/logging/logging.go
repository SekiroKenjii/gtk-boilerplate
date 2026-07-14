// Package logging builds the application's slog logger.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Options controls how the logger is built.
type Options struct {
	// Level is one of debug, info, warn, error. An unrecognised value falls
	// back to info rather than failing: a typo in an environment variable
	// should not stop the app from starting.
	Level string

	// JSON switches to structured output. Leave it off for a terminal, turn it
	// on when the app runs under a supervisor that parses logs.
	JSON bool
}

// New builds a logger writing to stderr.
//
// stderr, not stdout: a GUI app launched from a desktop file has its stdout
// captured by the session manager, and mixing logs into whatever the app might
// legitimately print makes both harder to read.
func New(opts Options) *slog.Logger {
	handlerOpts := &slog.HandlerOptions{Level: parseLevel(opts.Level)}

	var h slog.Handler
	if opts.JSON {
		h = slog.NewJSONHandler(os.Stderr, handlerOpts)
	} else {
		h = slog.NewTextHandler(os.Stderr, handlerOpts)
	}
	return slog.New(h)
}

// FromEnv builds a logger from APP_LOG_LEVEL and APP_LOG_JSON.
func FromEnv() *slog.Logger {
	return New(Options{
		Level: os.Getenv("APP_LOG_LEVEL"),
		JSON:  os.Getenv("APP_LOG_JSON") == "1",
	})
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
