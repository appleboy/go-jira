package oauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// genLoopbackCert writes a self-signed cert+key for 127.0.0.1 into a temp dir
// and returns the two file paths. It mirrors what a tool like mkcert produces
// for the loopback callback, so the HTTPS callback path can be exercised.
func genLoopbackCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "EC PRIVATE KEY", keyDER)
	return certPath, keyPath
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(
		&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// getCallbackTLS fires an OAuth redirect at an HTTPS callback server, skipping
// cert verification (the test cert is self-signed). Safe to call in a goroutine.
func getCallbackTLS(t *testing.T, port int, query string) {
	t.Helper()
	url := fmt.Sprintf("https://127.0.0.1:%d/callback?%s", port, query)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Errorf("build callback request: %v", err)
		return
	}
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("GET callback: %v", err)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// stubBrowserHittingCallback replaces browserCommand so that "opening the
// browser" instead extracts the state from the authorize URL and fires the
// OAuth redirect at the local callback server, simulating a user who approves.
// It returns an error so OpenBrowser never spawns a real process.
func stubBrowserHittingCallback(t *testing.T, port int, code string) {
	t.Helper()
	orig := browserCommand
	t.Cleanup(func() { browserCommand = orig })
	browserCommand = func(rawURL string) (string, []string, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Errorf("parse authorize url: %v", err)
		}
		state := u.Query().Get("state")
		go getCallback(t, port, fmt.Sprintf("code=%s&state=%s", code, url.QueryEscape(state)))
		return "", nil, errors.New("browser stubbed")
	}
}

func TestLoginEndToEnd(t *testing.T) {
	srv := tokenServer(t, func(_ *testing.T, w http.ResponseWriter, form map[string][]string) {
		if got := form["grant_type"]; len(got) != 1 || got[0] != "authorization_code" {
			t.Errorf("grant_type = %v", got)
		}
		if got := form["code"]; len(got) != 1 || got[0] != "browser-code" {
			t.Errorf("code = %v, want [browser-code]", got)
		}
		writeToken(w, oauth2.Token{AccessToken: "access-final"}, "refresh-final")
	})
	defer srv.Close()

	port := freePort(t)
	cfg := testConfig(srv.URL)
	cfg.RedirectURI = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	stubBrowserHittingCallback(t, port, "browser-code")

	res, err := Login(context.Background(), cfg, port, 5*time.Second)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Token.AccessToken != "access-final" {
		t.Errorf("access token = %q, want access-final", res.Token.AccessToken)
	}
	if res.Token.RefreshToken != "refresh-final" {
		t.Errorf("refresh token = %q, want refresh-final", res.Token.RefreshToken)
	}
}

// TestLoginEndToEndTLS exercises the full flow against an HTTPS callback server
// backed by a self-signed loopback cert, mirroring the Jira DC setup that
// requires an https redirect URI.
func TestLoginEndToEndTLS(t *testing.T) {
	srv := tokenServer(t, func(_ *testing.T, w http.ResponseWriter, form map[string][]string) {
		if got := form["code"]; len(got) != 1 || got[0] != "browser-code" {
			t.Errorf("code = %v, want [browser-code]", got)
		}
		writeToken(w, oauth2.Token{AccessToken: "access-final"}, "refresh-final")
	})
	defer srv.Close()

	certPath, keyPath := genLoopbackCert(t)
	port := freePort(t)
	cfg := testConfig(srv.URL)
	cfg.RedirectURI = fmt.Sprintf("https://127.0.0.1:%d/callback", port)
	cfg.TLSCertFile = certPath
	cfg.TLSKeyFile = keyPath

	// Browser stub fires the redirect at the HTTPS callback over a TLS client.
	orig := browserCommand
	t.Cleanup(func() { browserCommand = orig })
	browserCommand = func(rawURL string) (string, []string, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Errorf("parse authorize url: %v", err)
		}
		state := u.Query().Get("state")
		query := fmt.Sprintf("code=browser-code&state=%s", url.QueryEscape(state))
		go getCallbackTLS(t, port, query)
		return "", nil, errors.New("browser stubbed")
	}

	res, err := Login(context.Background(), cfg, port, 5*time.Second)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Token.AccessToken != "access-final" {
		t.Errorf("access token = %q, want access-final", res.Token.AccessToken)
	}
}

// TestLoginTLSSchemeMismatch verifies that when a TLS pair is configured the
// redirect URI must use https, not http.
func TestLoginTLSSchemeMismatch(t *testing.T) {
	certPath, keyPath := genLoopbackCert(t)
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://127.0.0.1:8765/callback"
	cfg.TLSCertFile = certPath
	cfg.TLSKeyFile = keyPath

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when TLS is set but redirect URI scheme is http")
	}
	if !strings.Contains(err.Error(), "scheme must be https") {
		t.Errorf("error = %q, want a scheme-mismatch message", err.Error())
	}
}

func TestLoginTimeout(t *testing.T) {
	port := freePort(t)
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Browser stub that never triggers a callback.
	orig := browserCommand
	t.Cleanup(func() { browserCommand = orig })
	browserCommand = func(string) (string, []string, error) {
		return "", nil, errors.New("browser stubbed")
	}

	_, err := Login(context.Background(), cfg, port, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestLoginInvalidConfig(t *testing.T) {
	_, err := Login(context.Background(), &Config{}, 8765, time.Second)
	if err == nil {
		t.Fatal("expected validation error for empty config")
	}
}

// TestLoginInvalidPort verifies Login rejects an out-of-range callback port
// (e.g. an explicit --callback-port=0) instead of trying to bind :0.
func TestLoginInvalidPort(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://127.0.0.1:0/callback"

	_, err := Login(context.Background(), cfg, 0, time.Second)
	if err == nil {
		t.Fatal("expected error for out-of-range callback port")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error = %q, want an out-of-range message", err.Error())
	}
}

// TestLoginRedirectSchemeMismatch verifies Login rejects an https redirect URI,
// since the callback server serves plain HTTP.
func TestLoginRedirectSchemeMismatch(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "https://127.0.0.1:8765/callback"

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when redirect URI scheme is not http")
	}
	if !strings.Contains(err.Error(), "scheme must be http") {
		t.Errorf("error = %q, want a scheme-mismatch message", err.Error())
	}
}

// TestLoginRedirectHostMismatch verifies Login rejects a redirect URI whose
// host is not the loopback address the callback server binds.
func TestLoginRedirectHostMismatch(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://localhost:8765/callback"

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when redirect URI host is not 127.0.0.1")
	}
	if !strings.Contains(err.Error(), "host must be 127.0.0.1") {
		t.Errorf("error = %q, want a host-mismatch message", err.Error())
	}
}

// TestLoginRedirectPathMismatch verifies Login rejects a redirect URI whose
// path is not the one the callback server serves.
func TestLoginRedirectPathMismatch(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://127.0.0.1:8765/wrong"

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when redirect URI path is not /callback")
	}
	if !strings.Contains(err.Error(), "path must be /callback") {
		t.Errorf("error = %q, want a path-mismatch message", err.Error())
	}
}

// TestLoginRedirectPortMismatch verifies Login fails fast (rather than hanging
// until timeout) when the callback port disagrees with the RedirectURI port.
func TestLoginRedirectPortMismatch(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://127.0.0.1:9999/callback"

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when redirect URI port does not match callback port")
	}
	if !strings.Contains(err.Error(), "does not match callback port") {
		t.Errorf("error = %q, want a port-mismatch message", err.Error())
	}
}
