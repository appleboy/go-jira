package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

// testConfig returns a Config pointed at srv with sensible defaults.
func testConfig(baseURL string) *Config {
	return &Config{
		BaseURL:      baseURL,
		ClientID:     "client-abc",
		ClientSecret: "secret-xyz",
		RedirectURI:  "http://127.0.0.1:8765/callback",
		Scopes:       []string{"WRITE"},
	}
}

// tokenServer spins up a mock Jira DC token endpoint. The supplied handler
// receives the parsed request form and writes the response.
type tokenHandler func(t *testing.T, w http.ResponseWriter, form map[string][]string)

func tokenServer(t *testing.T, handler tokenHandler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		handler(t, w, r.PostForm)
	})
	return httptest.NewServer(mux)
}

// writeToken is a convenience that emits a valid token JSON response.
func writeToken(w http.ResponseWriter, tok oauth2.Token, refreshToken string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  tok.AccessToken,
		"token_type":    "bearer",
		"expires_in":    7200,
		"refresh_token": refreshToken,
		"created_at":    1607635748,
	})
}

// writeOAuthError emits an RFC 6749 error response with the given status.
func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": desc,
	})
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid", func(*Config) {}, false},
		{"nil base url", func(c *Config) { c.BaseURL = "" }, true},
		{"nil client id", func(c *Config) { c.ClientID = "" }, true},
		{"nil redirect", func(c *Config) { c.RedirectURI = "" }, true},
		{"no scopes", func(c *Config) { c.Scopes = nil }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testConfig("https://jira.example.com")
			tt.mutate(c)
			if err := c.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	var nilCfg *Config
	if err := nilCfg.Validate(); err == nil {
		t.Error("nil config Validate() should error")
	}
}
