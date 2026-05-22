package oauth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// freePort returns a currently-free loopback TCP port. There is an inherent
// (small) race between closing and rebinding, acceptable for tests.
func freePort(t *testing.T) int {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// getCallback fires an OAuth redirect at the local callback server. It uses
// Errorf (not Fatalf) so it is safe to call from a goroutine.
func getCallback(t *testing.T, port int, query string) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/callback?%s", port, query)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Errorf("build callback request: %v", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("GET callback: %v", err)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func TestCallbackServer(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
		wantC   string
	}{
		{"success", "code=abc123&state=expected", false, "abc123"},
		{"state mismatch", "code=abc123&state=wrong", true, ""},
		{"provider error", "error=access_denied&error_description=nope", true, ""},
		{"missing code", "state=expected", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := freePort(t)
			resultCh, shutdown, err := startCallbackServer(port, "expected", "", "")
			if err != nil {
				t.Fatalf("startCallbackServer: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = shutdown(ctx)
			}()

			getCallback(t, port, tt.query)

			select {
			case res := <-resultCh:
				if tt.wantErr {
					if res.Err == nil {
						t.Errorf("expected error, got code %q", res.Code)
					}
					return
				}
				if res.Err != nil {
					t.Fatalf("unexpected error: %v", res.Err)
				}
				if res.Code != tt.wantC {
					t.Errorf("code = %q, want %q", res.Code, tt.wantC)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for callback result")
			}
		})
	}
}
