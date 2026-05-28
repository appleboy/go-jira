package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// runVersionCapture executes `version` with the given args against a fresh root
// command, capturing stdout.
func runVersionCapture(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	root.SetArgs(append([]string{"version"}, args...))
	var out bytes.Buffer
	root.SetOut(&out)
	err := root.Execute()
	return out.String(), err
}

// TestVersionTextOutput verifies the default (text) output names the binary and
// reports the build version.
func TestVersionTextOutput(t *testing.T) {
	out, err := runVersionCapture(t)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "go-jira version ") {
		t.Errorf("text output missing version line: %q", out)
	}
	if !strings.Contains(out, versionString()) {
		t.Errorf("text output missing version %q: %q", versionString(), out)
	}
}

// TestVersionJSONOutput verifies the JSON form parses and carries the build
// metadata an agent needs.
func TestVersionJSONOutput(t *testing.T) {
	out, err := runVersionCapture(t, "--output", outputJSON)
	if err != nil {
		t.Fatalf("version --output json: %v", err)
	}
	var info buildInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("version JSON did not parse: %v", err)
	}
	wantName := newRootCmd().Name()
	if info.Name != wantName {
		t.Errorf("name = %q, want %q", info.Name, wantName)
	}
	if info.Version != versionString() {
		t.Errorf("version = %q, want %q", info.Version, versionString())
	}
	if info.GoVersion == "" {
		t.Error("go_version is empty")
	}
	if !strings.Contains(info.Platform, "/") {
		t.Errorf("platform = %q, want OS/arch", info.Platform)
	}
}

// TestVersionInvalidOutput rejects an unknown --output value.
func TestVersionInvalidOutput(t *testing.T) {
	if _, err := runVersionCapture(t, "--output", "bogus"); err == nil {
		t.Fatal("expected error for invalid output format, got nil")
	}
}
