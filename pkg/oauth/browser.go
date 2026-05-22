package oauth

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// browserCommand returns the command and args to open rawURL in the user's
// default browser for the current OS, or an error on unsupported platforms.
// It is a variable so tests can stub it without spawning real processes.
var browserCommand = func(rawURL string) (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "linux":
		return "xdg-open", []string{rawURL}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "", nil, fmt.Errorf("oauth: unsupported platform %s", runtime.GOOS)
	}
}

// OpenBrowser tries to open rawURL in the user's default browser. On
// unsupported platforms or launch failure it returns an error; the caller is
// expected to print the URL as a fallback.
func OpenBrowser(rawURL string) error {
	name, args, err := browserCommand(rawURL)
	if err != nil {
		return err
	}
	// Background context: the browser process is fire-and-forget and must
	// outlive this call. Args are fixed and the URL is one we built.
	return exec.CommandContext(context.Background(), name, args...).Start() // #nosec G204
}
