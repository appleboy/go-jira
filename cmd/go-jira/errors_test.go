package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/appleboy/go-jira/pkg/oauth"
)

func TestClassifyPrefersExistingCLIError(t *testing.T) {
	want := &cliError{code: exitUsage, kind: "usage", message: "bad flag"}
	got := classify(fmt.Errorf("wrapped: %w", want), &requestDiag{})
	if got != want {
		t.Fatalf("classify did not return the wrapped *cliError: got %#v", got)
	}
}

func TestClassifyFromDiag(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		retryAfter string
		wantCode   int
		wantKind   string
		wantRetry  string
	}{
		{"rate limit", http.StatusTooManyRequests, "30", exitRateLimit, "rate_limit", "30"},
		{"unauthorized", http.StatusUnauthorized, "", exitAuth, "auth", ""},
		{"forbidden", http.StatusForbidden, "", exitAuth, "auth", ""},
		{"other 4xx falls through", http.StatusNotFound, "", exitError, "error", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &requestDiag{}
			d.record(tt.status, tt.retryAfter)
			ce := classify(errors.New("error searching issues: boom"), d)
			if ce.code != tt.wantCode || ce.kind != tt.wantKind {
				t.Fatalf("got code=%d kind=%q, want code=%d kind=%q",
					ce.code, ce.kind, tt.wantCode, tt.wantKind)
			}
			if ce.retryAfter != tt.wantRetry {
				t.Fatalf("retry_after = %q, want %q", ce.retryAfter, tt.wantRetry)
			}
			if tt.wantCode == exitRateLimit && ce.statusCode != tt.status {
				t.Fatalf("status_code = %d, want %d", ce.statusCode, tt.status)
			}
		})
	}
}

func TestClassifyFromMessage(t *testing.T) {
	tests := []struct {
		msg      string
		wantCode int
		wantKind string
	}{
		{`required flag(s) "jql" not set`, exitUsage, "usage"},
		{"unknown command \"foo\" for \"go-jira\"", exitUsage, "usage"},
		{`invalid output format "yaml": must be "json" or "text"`, exitUsage, "usage"},
		{"auth resolution: no credentials found", exitAuth, "auth"},
		{"auth validation: token rejected", exitAuth, "auth"},
		{"error creating jira client: parse url", exitError, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			ce := classify(errors.New(tt.msg), nil)
			if ce.code != tt.wantCode || ce.kind != tt.wantKind {
				t.Fatalf("got code=%d kind=%q, want code=%d kind=%q",
					ce.code, ce.kind, tt.wantCode, tt.wantKind)
			}
		})
	}
}

// TestClassifyInvalidGrantIsAuth verifies a refresh failure wrapping
// oauth.ErrInvalidGrant is classified as an auth failure (exit 3) by error
// identity — it carries no "auth ..." message prefix or HTTP diagnostics, so
// only errors.Is can catch it. This is what makes `token refresh` exit 3 (not
// 1) and pick up the auth recovery hint when the refresh token is dead.
func TestClassifyInvalidGrantIsAuth(t *testing.T) {
	err := fmt.Errorf("refresh failed: %w", oauth.ErrInvalidGrant)
	ce := classify(err, &requestDiag{})
	if ce.code != exitAuth || ce.kind != kindAuth {
		t.Fatalf("got code=%d kind=%q, want code=%d kind=%q",
			ce.code, ce.kind, exitAuth, kindAuth)
	}
}

func TestClassifyNilReturnsNil(t *testing.T) {
	if classify(nil, &requestDiag{}) != nil {
		t.Fatal("classify(nil) should return nil")
	}
}

// TestAddHintPopulatesActionableGuidance checks that addHint attaches a
// kind-appropriate next step, and that an unknown command surfaces a spelling
// suggestion drawn from the registered subcommands.
func TestAddHintPopulatesActionableGuidance(t *testing.T) {
	root := newRootCmd()

	tests := []struct {
		name     string
		ce       *cliError
		contains string
	}{
		{
			"unknown command suggests nearest match",
			&cliError{kind: kindUsage, message: `unknown command "serch" for "go-jira"`},
			`Did you mean "search"?`,
		},
		{
			"generic usage points at help",
			&cliError{kind: kindUsage, message: `required flag(s) "jql" not set`},
			"go-jira --help",
		},
		{
			"auth points at the token refresh recovery step, not back at the failed command",
			&cliError{kind: kindAuth, message: "auth resolution: x"},
			"token refresh",
		},
		{
			"rate limit references retry_after",
			&cliError{kind: kindRateLimit, message: "slow down"},
			"retry_after",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addHint(tt.ce, root)
			if tt.ce.hint == "" || !strings.Contains(tt.ce.hint, tt.contains) {
				t.Fatalf("hint = %q, want it to contain %q", tt.ce.hint, tt.contains)
			}
		})
	}
}

// TestAddHintKeepsExistingHint verifies addHint never overwrites a hint that was
// already set on the error.
func TestAddHintKeepsExistingHint(t *testing.T) {
	ce := &cliError{kind: kindUsage, message: "x", hint: "preset"}
	addHint(ce, newRootCmd())
	if ce.hint != "preset" {
		t.Fatalf("hint = %q, want it left untouched", ce.hint)
	}
}

// TestDiagTransportRecordsErrorStatus verifies the transport captures the status
// and Retry-After of a 4xx response into the requestDiag carried by the request
// context, and leaves 2xx responses unrecorded.
func TestDiagTransportRecordsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	d := &requestDiag{}
	client := &http.Client{Transport: &diagTransport{base: http.DefaultTransport}}

	doGet := func(path string) {
		req, err := http.NewRequestWithContext(
			withDiag(context.Background(), d), http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		resp.Body.Close()
	}

	doGet("/ok")
	if sc, _ := d.snapshot(); sc != 0 {
		t.Fatalf("2xx should not be recorded, got status %d", sc)
	}

	doGet("/limited")
	sc, retryAfter := d.snapshot()
	if sc != http.StatusTooManyRequests || retryAfter != "42" {
		t.Fatalf("got status=%d retry=%q, want 429/42", sc, retryAfter)
	}
}
