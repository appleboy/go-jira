package oauth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
	"time"
)

// callbackPath is the single path the callback server serves and the only path
// a redirect URI may use; Login validates the redirect URI against it so a
// mismatch fails fast instead of hanging until timeout.
const callbackPath = "/callback"

// callbackResult carries the outcome of the OAuth redirect back to the caller.
type callbackResult struct {
	Code  string
	State string
	Err   error
}

// startCallbackServer starts a server on 127.0.0.1:port that listens for the
// OAuth redirect. It returns a channel that receives exactly one result, and a
// shutdown function the caller MUST call (typically via defer).
//
// When certFile and keyFile are both non-empty the server serves HTTPS using
// that key pair (for Jira DC, which requires an https redirect URI); otherwise
// it serves plain HTTP.
func startCallbackServer(
	port int,
	expectedState string,
	certFile, keyFile string,
) (<-chan callbackResult, func(context.Context) error, error) {
	resultCh := make(chan callbackResult, 1)

	// send delivers the first result and ignores any later callback hits
	// (browser refresh, back button, multiple tabs). Without this the handler
	// goroutine would block forever on the second send into the 1-slot channel.
	var once sync.Once
	send := func(res callbackResult) {
		once.Do(func() { resultCh <- res })
	}

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errStr := q.Get("error"); errStr != "" {
			desc := q.Get("error_description")
			writeCallbackHTML(w, false, desc)
			send(callbackResult{
				Err: fmt.Errorf("oauth: callback error: %s (%s)", errStr, desc),
			})
			return
		}
		if state := q.Get("state"); state != expectedState {
			writeCallbackHTML(w, false, "state mismatch (possible CSRF)")
			send(callbackResult{Err: errors.New("oauth: state mismatch")})
			return
		}
		code := q.Get("code")
		if code == "" {
			writeCallbackHTML(w, false, "no code in callback")
			send(callbackResult{Err: errors.New("oauth: no code in callback")})
			return
		}
		writeCallbackHTML(w, true, "")
		send(callbackResult{Code: code, State: q.Get("state")})
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("oauth: listen %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if certFile != "" || keyFile != "" {
		// Load the key pair before serving so a bad/missing cert fails Login
		// synchronously here, instead of being swallowed by the Serve goroutine
		// and surfacing only as a connection-refused redirect that hangs until
		// timeout. Config.Validate already rejects a half-set pair.
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			_ = ln.Close()
			return nil, nil, fmt.Errorf("oauth: load TLS key pair: %w", err)
		}
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		// Certs are already loaded into TLSConfig, so the file args are empty.
		go func() { _ = srv.ServeTLS(ln, "", "") }()
		return resultCh, srv.Shutdown, nil
	}

	go func() { _ = srv.Serve(ln) }()
	return resultCh, srv.Shutdown, nil
}

// writeCallbackHTML renders a minimal success/failure page for the browser.
func writeCallbackHTML(w http.ResponseWriter, success bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if success {
		_, _ = w.Write([]byte(callbackSuccessHTML))
		return
	}
	_, _ = fmt.Fprintf(w, callbackErrorHTML, html.EscapeString(msg))
}

const callbackSuccessHTML = `<!doctype html><html><head>
<meta charset="utf-8"><title>Login successful</title>
<style>body{font-family:system-ui;text-align:center;padding:3em;}</style>
</head><body><h1>✅ Login successful</h1>
<p>You can close this window and return to your terminal.</p>
</body></html>`

const callbackErrorHTML = `<!doctype html><html><head>
<meta charset="utf-8"><title>Login failed</title>
<style>body{font-family:system-ui;text-align:center;padding:3em;color:#c00;}</style>
</head><body><h1>❌ Login failed</h1>
<p>%s</p><p>Return to your terminal for details.</p>
</body></html>`
