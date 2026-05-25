package main

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// newTestHandler builds a cliHandler writing to buf at the given level/color so
// tests can assert on the rendered output.
func newTestHandler(buf *bytes.Buffer, level slog.Level, color bool) *cliHandler {
	return &cliHandler{mu: &sync.Mutex{}, w: buf, level: level, color: color}
}

func TestCliHandlerQuietSuppressesInfo(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(newTestHandler(&buf, slog.LevelWarn, false))

	log.Info("hello info")
	log.Warn("hello warn")

	out := buf.String()
	if strings.Contains(out, "hello info") {
		t.Errorf("quiet handler should drop Info logs, got:\n%s", out)
	}
	if !strings.Contains(out, "hello warn") {
		t.Errorf("quiet handler should keep Warn logs, got:\n%s", out)
	}
}

func TestCliHandlerNoColorWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(newTestHandler(&buf, slog.LevelInfo, false))
	log.Info("plain", "key", "value")

	out := buf.String()
	if strings.Contains(out, "\033[") {
		t.Errorf("color disabled but ANSI escape present, got:\n%q", out)
	}
	if !strings.Contains(out, "INFO plain key=value") {
		t.Errorf("unexpected rendering, got:\n%q", out)
	}
}

func TestCliHandlerColorWrapsLevel(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(newTestHandler(&buf, slog.LevelInfo, true))
	log.Info("colored")

	out := buf.String()
	if !strings.Contains(out, ansiCyan) || !strings.Contains(out, ansiReset) {
		t.Errorf("color enabled but level not wrapped, got:\n%q", out)
	}
}

func TestCliHandlerQuotesValuesWithSpaces(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(newTestHandler(&buf, slog.LevelInfo, false))
	log.Info("msg", "path", "/tmp/a b")

	if !strings.Contains(buf.String(), `path="/tmp/a b"`) {
		t.Errorf("expected quoted value, got:\n%q", buf.String())
	}
}

func TestCliHandlerWithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(newTestHandler(&buf, slog.LevelInfo, false))
	log := base.With("a", 1).WithGroup("g")
	log.Info("msg", "b", 2)

	out := buf.String()
	if !strings.Contains(out, "a=1") || !strings.Contains(out, "g.b=2") {
		t.Errorf("WithAttrs/WithGroup not rendered correctly, got:\n%q", out)
	}
}

func TestColorEnabled(t *testing.T) {
	if colorEnabled(true) {
		t.Error("colorEnabled(true) should be false when --no-color is set")
	}
	t.Setenv("NO_COLOR", "1")
	if colorEnabled(false) {
		t.Error("colorEnabled should be false when NO_COLOR is set")
	}
}

func TestSetupLoggingDoesNotPanic(t *testing.T) {
	// Restore the default logger after mutating the global.
	t.Cleanup(func() { slog.SetDefault(slog.Default()) })
	setupLogging(true, true)
	slog.Info("suppressed") // should be dropped (quiet)
	slog.Warn("kept")       // should pass the threshold
}
