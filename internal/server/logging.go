package server

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// NewLogger returns a text logger appending to logPath and, when stderr is not nil, also writing to stderr. Close the
// returned closer on exit.
func NewLogger(logPath string, stderr io.Writer) (*slog.Logger, io.Closer, error) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // Operator's path.
	if err != nil {
		return nil, nil, fmt.Errorf("open log: %w", err)
	}
	var w io.Writer = f
	if stderr != nil {
		w = io.MultiWriter(f, stderr)
	}
	return slog.New(slog.NewTextHandler(w, nil)), f, nil
}
