package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ANSI escape sequences used to colorize the level token when color is enabled.
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[90m"
)

// setupLogging installs the process-wide slog handler used for the human-facing
// diagnostics every command writes to stderr (the machine-readable result goes
// to stdout). It is called once from the root PersistentPreRunE so the flags are
// honored before any subcommand logs.
//
//   - quiet raises the threshold to Warn, so the informational "authenticated",
//     "user account", etc. lines are suppressed while warnings, errors, and the
//     result on stdout remain.
//   - color is enabled only when neither --no-color nor NO_COLOR is set and
//     stderr is a terminal, matching the https://no-color.org convention.
func setupLogging(quiet, noColor bool) {
	level := slog.LevelInfo
	if quiet {
		level = slog.LevelWarn
	}
	slog.SetDefault(slog.New(&cliHandler{
		mu:    &sync.Mutex{},
		w:     os.Stderr,
		level: level,
		color: colorEnabled(noColor),
	}))
}

// colorEnabled reports whether ANSI color should be emitted. Color is off when
// the caller passed --no-color, the NO_COLOR env var is present (any value, per
// no-color.org), or stderr is not a character device (file/pipe).
func colorEnabled(noColor bool) bool {
	if noColor {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// cliHandler is a compact slog.Handler that writes "LEVEL message key=value"
// lines, optionally colorizing the level token. The stdlib text/JSON handlers
// never emit color, so a small custom handler is what makes --no-color/NO_COLOR
// meaningful while keeping the existing terse diagnostics format.
type cliHandler struct {
	mu           *sync.Mutex
	w            io.Writer
	level        slog.Level
	color        bool
	groups       []string
	preformatted string // attrs accumulated via WithAttrs, already rendered
}

func (h *cliHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *cliHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(h.levelToken(r.Level))
	b.WriteByte(' ')
	b.WriteString(r.Message)
	b.WriteString(h.preformatted)
	prefix := strings.Join(h.groups, ".")
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(&b, prefix, a)
		return true
	})
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *cliHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()
	var b strings.Builder
	prefix := strings.Join(h.groups, ".")
	for _, a := range attrs {
		appendAttr(&b, prefix, a)
	}
	h2.preformatted = h.preformatted + b.String()
	return h2
}

func (h *cliHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(append([]string{}, h.groups...), name)
	return h2
}

func (h *cliHandler) clone() *cliHandler {
	c := *h
	return &c
}

// levelToken returns the uppercase level label, wrapped in an ANSI color when
// color output is enabled.
func (h *cliHandler) levelToken(l slog.Level) string {
	label := l.String()
	if !h.color {
		return label
	}
	var c string
	switch {
	case l >= slog.LevelError:
		c = ansiRed
	case l >= slog.LevelWarn:
		c = ansiYellow
	case l >= slog.LevelInfo:
		c = ansiCyan
	default:
		c = ansiGray
	}
	return c + label + ansiReset
}

// appendAttr renders a single attribute as " key=value", quoting the value when
// it contains whitespace or delimiters. Group attributes recurse with a
// dot-joined key prefix.
func appendAttr(b *strings.Builder, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			appendAttr(b, key, ga)
		}
		return
	}
	b.WriteByte(' ')
	b.WriteString(key)
	b.WriteByte('=')
	val := a.Value.String()
	if val == "" || strings.ContainsAny(val, " \t\"=") {
		val = strconv.Quote(val)
	}
	b.WriteString(val)
}
