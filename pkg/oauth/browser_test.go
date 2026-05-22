package oauth

import (
	"errors"
	"runtime"
	"testing"
)

func TestBrowserCommand(t *testing.T) {
	name, args, err := browserCommand("https://example.com")
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		if err != nil {
			t.Fatalf("unexpected error on %s: %v", runtime.GOOS, err)
		}
		if name == "" {
			t.Error("expected a non-empty command name")
		}
		if len(args) == 0 {
			t.Error("expected at least one arg")
		}
	default:
		if err == nil {
			t.Errorf("expected unsupported-platform error on %s", runtime.GOOS)
		}
	}
}

func TestOpenBrowserPropagatesCommandError(t *testing.T) {
	orig := browserCommand
	t.Cleanup(func() { browserCommand = orig })

	sentinel := errors.New("stubbed")
	browserCommand = func(string) (string, []string, error) {
		return "", nil, sentinel
	}
	if err := OpenBrowser("https://example.com"); !errors.Is(err, sentinel) {
		t.Errorf("OpenBrowser error = %v, want %v", err, sentinel)
	}
}
