package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestEpicsCmd(t *testing.T) {
	// The handler runs on a separate goroutine from the test body, so guard the
	// captured request details with a mutex to stay clean under `go test -race`.
	var mu sync.Mutex
	var gotPath, gotQuery string
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"maxResults": 50,
				"startAt":    0,
				"isLast":     true,
				"values": []map[string]any{
					{
						"id":      1,
						"key":     "GAIA-100",
						"name":    "Login epic",
						"summary": "Login",
						"done":    false,
					},
					{
						"id":      2,
						"key":     "GAIA-200",
						"name":    "Billing epic",
						"summary": "Billing",
						"done":    false,
					},
				},
			})
		}),
	)
	defer server.Close()

	// Happy path: JSON output contains both epics, and the request hits the
	// Agile board-epic endpoint built by jira.BoardService.GetEpics.
	out, err := runDataCmd(t, newEpicsCmd(), server.URL, "--board-id", "10381")
	if err != nil {
		t.Fatalf("epics returned error: %v", err)
	}
	if !strings.Contains(out, "GAIA-100") || !strings.Contains(out, "GAIA-200") {
		t.Errorf("epics output missing epics: %s", out)
	}
	mu.Lock()
	path, query := gotPath, gotQuery
	mu.Unlock()
	if path != "/rest/agile/1.0/board/10381/epic" {
		t.Errorf("unexpected request path: %s", path)
	}
	// done=false is always sent (active epics); maxResults defaults to 50.
	if !strings.Contains(query, "done=false") || !strings.Contains(query, "maxResults=50") {
		t.Errorf("unexpected query string: %s", query)
	}

	// A custom --limit flows through to maxResults in the query string.
	if _, err := runDataCmd(
		t,
		newEpicsCmd(),
		server.URL,
		"--board-id",
		"10381",
		"--limit",
		"20",
	); err != nil {
		t.Fatalf("epics with limit returned error: %v", err)
	}
	mu.Lock()
	query = gotQuery
	mu.Unlock()
	if !strings.Contains(query, "maxResults=20") {
		t.Errorf("expected maxResults=20 in query, got: %s", query)
	}

	// Text output: one tab-separated line per epic.
	out, err = runDataCmd(t, newEpicsCmd(), server.URL, "--board-id", "10381", "--output", "text")
	if err != nil {
		t.Fatalf("epics text returned error: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n"); lines != 1 {
		t.Errorf("expected 2 text lines, got %d: %q", lines+1, out)
	}
}

func TestEpicsCmdMissingBoardID(t *testing.T) {
	// --board-id is required; cobra should reject before any request.
	_, err := runDataCmd(t, newEpicsCmd(), "https://example.invalid")
	if err == nil {
		t.Fatal("expected error for missing --board-id")
	}
}

func TestEpicsCmdServerError(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorMessages":["unauthorized"]}`))
		}),
	)
	defer server.Close()

	_, err := runDataCmd(t, newEpicsCmd(), server.URL, "--board-id", "10381")
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}
