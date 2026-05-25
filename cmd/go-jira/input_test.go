package main

import (
	"strings"
	"testing"
)

func TestResolveStdinPassesThroughNonSentinel(t *testing.T) {
	got, err := resolveStdin("literal value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "literal value" {
		t.Errorf("expected literal value unchanged, got %q", got)
	}
}

func TestResolveStdinReadsAndTrimsTrailingNewline(t *testing.T) {
	orig := stdinReader
	t.Cleanup(func() { stdinReader = orig })
	stdinReader = strings.NewReader("piped body\n")

	got, err := resolveStdin(stdinSentinel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "piped body" {
		t.Errorf("expected trailing newline trimmed, got %q", got)
	}
}

func TestResolveStdinEmptyInput(t *testing.T) {
	orig := stdinReader
	t.Cleanup(func() { stdinReader = orig })
	stdinReader = strings.NewReader("")

	got, err := resolveStdin(stdinSentinel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
