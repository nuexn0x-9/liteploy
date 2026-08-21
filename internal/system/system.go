// Package system provides application-wide infrastructure: logger setup,
// version info, and OS signal handling helpers.
package system

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
)

// Version information, injected at build time via -ldflags.
var (
	Version   = "v1.0.0"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

// NewLogger creates a structured slog logger configured for the given level and format.
// json=true outputs JSON (for production log aggregators), json=false outputs human-readable text.
func NewLogger(level string, json bool) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if json {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// GenerateSecret generates a cryptographically random hex string of the given byte length.
// Used for generating session secrets in dev mode.
// Never use this output as a persistent secret — it changes on every restart.
func GenerateSecret(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
