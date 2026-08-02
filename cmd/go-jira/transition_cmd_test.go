package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

const testTransitionPath = "/rest/api/2/issue/GAIA-123/transitions"

type testTransitionListResult struct {
	Key         string               `json:"key"`
	Transitions []testTransitionView `json:"transitions"`
}

type testTransitionView struct {
	ID     string                          `json:"id"`
	Name   string                          `json:"name"`
	To     string                          `json:"to"`
	Fields map[string]jira.TransitionField `json:"fields"`
}

type testTransitionExecuteResult struct {
	Status string `json:"status"`
	Key    string `json:"key"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	To     string `json:"to"`
}

type testTransitionPayload struct {
	Transition struct {
		ID string `json:"id"`
	} `json:"transition"`
	Fields struct {
		Resolution *struct {
			ID string `json:"id"`
		} `json:"resolution"`
	} `json:"fields"`
}

func testAvailableTransitions() []jira.Transition {
	return []jira.Transition{
		{
			ID:   "31",
			Name: "Done",
			To:   jira.Status{ID: "10002", Name: "Done"},
			Fields: map[string]jira.TransitionField{
				"resolution": {Required: true},
			},
		},
	}
}

func writeTestTransitions(t *testing.T, w http.ResponseWriter, transitions []jira.Transition) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"transitions": transitions}); err != nil {
		t.Errorf("encode transitions response: %v", err)
	}
}

func assertTestTransitionGET(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodGet {
		t.Errorf("transition lookup method = %s, want GET", r.Method)
	}
	if r.URL.Path != testTransitionPath {
		t.Errorf("transition lookup path = %q, want %q", r.URL.Path, testTransitionPath)
	}
	if got := r.URL.Query().Get("expand"); got != "transitions.fields" {
		t.Errorf("transition lookup expand = %q, want %q", got, "transitions.fields")
	}
}

func decodeTestTransitionPayload(t *testing.T, r *http.Request) testTransitionPayload {
	t.Helper()
	var payload testTransitionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Errorf("decode transition payload: %v", err)
	}
	return payload
}

func runTransitionTestCmd(
	t *testing.T,
	cmd *cobra.Command,
	serverURL string,
	args ...string,
) (string, error) {
	t.Helper()
	// The data-command harness already neutralizes output/auth-related variables.
	// Clear the two values unique to this command as well so a developer's shell
	// cannot silently add a resolution or select a legacy run transition.
	for _, key := range []string{"RESOLUTION", "INPUT_RESOLUTION", "TRANSITION", "INPUT_TRANSITION"} {
		t.Setenv(key, "")
	}
	return runDataCmd(t, cmd, serverURL, args...)
}

func TestTransitionListCmdJSONAndText(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		assert func(t *testing.T, out string)
	}{
		{
			name: "json",
			args: []string{"--key", "GAIA-123"},
			assert: func(t *testing.T, out string) {
				t.Helper()
				var got testTransitionListResult
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatalf("decode list JSON %q: %v", out, err)
				}
				if got.Key != "GAIA-123" {
					t.Errorf("list key = %q, want GAIA-123", got.Key)
				}
				if len(got.Transitions) != 1 {
					t.Fatalf("list transitions = %+v, want one entry", got.Transitions)
				}
				tr := got.Transitions[0]
				if tr.ID != "31" || tr.Name != "Done" || tr.To != "Done" {
					t.Errorf("list transition = %+v, want 31/Done -> Done", tr)
				}
				field, ok := tr.Fields["resolution"]
				if !ok || !field.Required {
					t.Errorf("list fields = %+v, want required resolution metadata", tr.Fields)
				}
			},
		},
		{
			name: "text",
			args: []string{"--key", "GAIA-123", "--output", "text"},
			assert: func(t *testing.T, out string) {
				t.Helper()
				if out != "31\tDone\tDone\n" {
					t.Errorf("list text = %q, want tab-separated transition row", out)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getCount := 0
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					getCount++
					assertTestTransitionGET(t, r)
					writeTestTransitions(t, w, testAvailableTransitions())
				}),
			)
			defer server.Close()

			out, err := runTransitionTestCmd(t, newTransitionListCmd(), server.URL, tt.args...)
			if err != nil {
				t.Fatalf("transition list returned error: %v", err)
			}
			if getCount != 1 {
				t.Errorf("transition lookup count = %d, want 1", getCount)
			}
			tt.assert(t, out)
		})
	}
}

func TestTransitionListCmdEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTestTransitionGET(t, r)
		writeTestTransitions(t, w, []jira.Transition{})
	}))
	defer server.Close()

	out, err := runTransitionTestCmd(t, newTransitionListCmd(), server.URL,
		"--key", "GAIA-123")
	if err != nil {
		t.Fatalf("empty transition list returned error: %v", err)
	}
	var got testTransitionListResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode empty list JSON %q: %v", out, err)
	}
	if got.Transitions == nil {
		t.Errorf("empty list transitions = null, want []")
	}
	if got.Key != "GAIA-123" || len(got.Transitions) != 0 {
		t.Errorf("empty list result = %+v, want GAIA-123 with no transitions", got)
	}
}

func TestTransitionListCmdNormalizesNilFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTestTransitionGET(t, r)
		writeTestTransitions(t, w, []jira.Transition{
			{
				ID:   "31",
				Name: "Done",
				To:   jira.Status{ID: "10002", Name: "Done"},
			},
		})
	}))
	defer server.Close()

	out, err := runTransitionTestCmd(t, newTransitionListCmd(), server.URL,
		"--key", "GAIA-123")
	if err != nil {
		t.Fatalf("transition list with nil fields returned error: %v", err)
	}
	var got testTransitionListResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode transition list JSON %q: %v", out, err)
	}
	if len(got.Transitions) != 1 {
		t.Fatalf("list transitions = %+v, want one entry", got.Transitions)
	}
	if got.Transitions[0].Fields == nil {
		t.Errorf("list fields = null, want an empty object")
	}
	if len(got.Transitions[0].Fields) != 0 {
		t.Errorf("list fields = %+v, want an empty object", got.Transitions[0].Fields)
	}
}

func TestTransitionExecuteCmdByIDAndName(t *testing.T) {
	tests := []struct {
		name       string
		selector   string
		outputArgs []string
		assertOut  func(t *testing.T, out string)
	}{
		{
			name:     "exact ID with JSON output",
			selector: "31",
			assertOut: func(t *testing.T, out string) {
				t.Helper()
				var got testTransitionExecuteResult
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatalf("decode execute JSON %q: %v", out, err)
				}
				want := testTransitionExecuteResult{
					Status: "transitioned", Key: "GAIA-123", ID: "31", Name: "Done", To: "Done",
				}
				if got != want {
					t.Errorf("execute result = %+v, want %+v", got, want)
				}
			},
		},
		{
			name:       "case-insensitive name with text output",
			selector:   "dOnE",
			outputArgs: []string{"--output", "text"},
			assertOut: func(t *testing.T, out string) {
				t.Helper()
				if out != "transitioned GAIA-123 via 31 (Done) -> Done\n" {
					t.Errorf("execute text = %q", out)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestOrder []string
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requestOrder = append(requestOrder, r.Method)
					switch r.Method {
					case http.MethodGet:
						assertTestTransitionGET(t, r)
						writeTestTransitions(t, w, testAvailableTransitions())
					case http.MethodPost:
						if r.URL.Path != testTransitionPath {
							t.Errorf(
								"transition POST path = %q, want %q",
								r.URL.Path,
								testTransitionPath,
							)
						}
						payload := decodeTestTransitionPayload(t, r)
						if payload.Transition.ID != "31" {
							t.Errorf("transition payload ID = %q, want 31", payload.Transition.ID)
						}
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Errorf("unexpected request method %s", r.Method)
						w.WriteHeader(http.StatusMethodNotAllowed)
					}
				}),
			)
			defer server.Close()

			args := []string{"--key", "GAIA-123", "--transition", tt.selector}
			args = append(args, tt.outputArgs...)
			out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL, args...)
			if err != nil {
				t.Fatalf("transition execute returned error: %v", err)
			}
			if strings.Join(requestOrder, ",") != "GET,POST" {
				t.Errorf("request order = %v, want GET then POST", requestOrder)
			}
			tt.assertOut(t, out)
		})
	}
}

func TestTransitionExecuteCmdSelectorValidation(t *testing.T) {
	t.Run("empty available list does not POST", func(t *testing.T) {
		postCount := 0
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					postCount++
					w.WriteHeader(http.StatusNoContent)
					return
				}
				assertTestTransitionGET(t, r)
				writeTestTransitions(t, w, []jira.Transition{})
			}),
		)
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "Done")
		if err == nil {
			t.Fatal("expected unavailable selector error for empty transition list")
		}
		if out != "" || postCount != 0 {
			t.Errorf("empty list produced stdout=%q POSTs=%d, want neither", out, postCount)
		}
		if !strings.Contains(err.Error(), "Done") || !strings.Contains(err.Error(), "none") {
			t.Errorf("empty-list selector error = %q, want selector and no choices", err)
		}
	})

	t.Run("unavailable selector does not POST", func(t *testing.T) {
		postCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				assertTestTransitionGET(t, r)
				writeTestTransitions(t, w, testAvailableTransitions())
			case http.MethodPost:
				postCount++
				w.WriteHeader(http.StatusNoContent)
			}
		}))
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "Blocked")
		if err == nil {
			t.Fatal("expected unavailable selector error")
		}
		if out != "" {
			t.Errorf("unavailable selector stdout = %q, want empty", out)
		}
		if postCount != 0 {
			t.Errorf("POST count = %d, want 0", postCount)
		}
		for _, want := range []string{"Blocked", "31", "Done"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("unavailable selector error = %q, want %q", err, want)
			}
		}
	})

	t.Run("duplicate name is ambiguous and does not POST", func(t *testing.T) {
		transitions := []jira.Transition{
			{ID: "31", Name: "Done", To: jira.Status{Name: "Done"}},
			{ID: "41", Name: "DONE", To: jira.Status{Name: "Closed"}},
		}
		postCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				postCount++
				w.WriteHeader(http.StatusNoContent)
				return
			}
			assertTestTransitionGET(t, r)
			writeTestTransitions(t, w, transitions)
		}))
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "done")
		if err == nil {
			t.Fatal("expected ambiguous transition name error")
		}
		if out != "" || postCount != 0 {
			t.Errorf("ambiguous name produced stdout=%q POSTs=%d, want neither", out, postCount)
		}
		for _, want := range []string{"ambiguous", "31", "41", "ID"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ambiguous name error = %q, want %q", err, want)
			}
		}
	})

	t.Run("exact ID wins over matching name", func(t *testing.T) {
		transitions := []jira.Transition{
			{ID: "Done", Name: "By ID", To: jira.Status{Name: "Selected by ID"}},
			{ID: "41", Name: "done", To: jira.Status{Name: "Selected by name"}},
		}
		postedID := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				assertTestTransitionGET(t, r)
				writeTestTransitions(t, w, transitions)
				return
			}
			postedID = decodeTestTransitionPayload(t, r).Transition.ID
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		_, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "Done")
		if err != nil {
			t.Fatalf("exact ID selector returned error: %v", err)
		}
		if postedID != "Done" {
			t.Errorf("posted transition ID = %q, want exact ID Done", postedID)
		}
	})
}

func TestTransitionExecuteCmdResolution(t *testing.T) {
	t.Run("optional resolution name is resolved into POST payload", func(t *testing.T) {
		var requestOrder []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestOrder = append(requestOrder, r.Method+" "+r.URL.Path)
			switch {
			case r.Method == http.MethodGet && r.URL.Path == testTransitionPath:
				assertTestTransitionGET(t, r)
				writeTestTransitions(t, w, testAvailableTransitions())
			case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/resolution":
				_ = json.NewEncoder(w).Encode([]jira.Resolution{{ID: "10000", Name: "Fixed"}})
			case r.Method == http.MethodPost && r.URL.Path == testTransitionPath:
				payload := decodeTestTransitionPayload(t, r)
				if payload.Transition.ID != "31" {
					t.Errorf("transition payload ID = %q, want 31", payload.Transition.ID)
				}
				if payload.Fields.Resolution == nil || payload.Fields.Resolution.ID != "10000" {
					t.Errorf("resolution payload = %+v, want ID 10000", payload.Fields.Resolution)
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "Done", "--resolution", "fixed")
		if err != nil {
			t.Fatalf("transition with resolution returned error: %v", err)
		}
		if !strings.Contains(out, `"status": "transitioned"`) {
			t.Errorf("transition with resolution output = %q", out)
		}
		wantOrder := []string{
			"GET " + testTransitionPath,
			"GET /rest/api/2/resolution",
			"POST " + testTransitionPath,
		}
		if strings.Join(requestOrder, "|") != strings.Join(wantOrder, "|") {
			t.Errorf("request order = %v, want %v", requestOrder, wantOrder)
		}
	})

	t.Run("unknown resolution fails before POST", func(t *testing.T) {
		postCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == testTransitionPath:
				assertTestTransitionGET(t, r)
				writeTestTransitions(t, w, testAvailableTransitions())
			case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/resolution":
				_ = json.NewEncoder(w).Encode([]jira.Resolution{{ID: "10000", Name: "Fixed"}})
			case r.Method == http.MethodPost:
				postCount++
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "31", "--resolution", "Missing")
		if err == nil {
			t.Fatal("expected unknown resolution error")
		}
		if out != "" || postCount != 0 {
			t.Errorf("unknown resolution produced stdout=%q POSTs=%d, want neither", out, postCount)
		}
		if !strings.Contains(err.Error(), "Missing") ||
			!strings.Contains(err.Error(), "not found") {
			t.Errorf("unknown resolution error = %q", err)
		}
	})
}

func TestTransitionCommandsSurfaceJiraErrors(t *testing.T) {
	t.Run("GET error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertTestTransitionGET(t, r)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errorMessages":["lookup failed"]}`))
		}))
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionListCmd(), server.URL,
			"--key", "GAIA-123")
		if err == nil {
			t.Fatal("expected transition GET error")
		}
		if out != "" {
			t.Errorf("GET error stdout = %q, want empty", out)
		}
		for _, want := range []string{"GAIA-123", "500", "lookup failed"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("GET error = %q, want %q", err, want)
			}
		}
	})

	t.Run("POST error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				assertTestTransitionGET(t, r)
				writeTestTransitions(t, w, testAvailableTransitions())
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errorMessages":["Invalid transition"]}`))
		}))
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "Done")
		if err == nil {
			t.Fatal("expected transition POST error")
		}
		if out != "" {
			t.Errorf("POST error stdout = %q, want empty", out)
		}
		for _, want := range []string{"GAIA-123", "400", "Invalid transition"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("POST error = %q, want %q", err, want)
			}
		}
	})

	t.Run("non-204 success status is rejected", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				assertTestTransitionGET(t, r)
				writeTestTransitions(t, w, testAvailableTransitions())
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		out, err := runTransitionTestCmd(t, newTransitionExecuteCmd(), server.URL,
			"--key", "GAIA-123", "--transition", "31")
		if err == nil {
			t.Fatal("expected non-204 transition error")
		}
		if out != "" {
			t.Errorf("non-204 stdout = %q, want empty", out)
		}
		if !strings.Contains(err.Error(), "200") || !strings.Contains(err.Error(), "GAIA-123") {
			t.Errorf("non-204 error = %q, want issue key and HTTP status", err)
		}
	})
}

func TestTransitionCommandsRequireFlags(t *testing.T) {
	tests := []struct {
		name     string
		cmd      func() *cobra.Command
		args     []string
		wantFlag string
	}{
		{
			name: "list requires key", cmd: newTransitionListCmd,
			wantFlag: flagKey,
		},
		{
			name: "execute requires key", cmd: newTransitionExecuteCmd,
			args: []string{"--transition", "Done"}, wantFlag: flagKey,
		},
		{
			name: "execute requires transition", cmd: newTransitionExecuteCmd,
			args: []string{"--key", "GAIA-123"}, wantFlag: flagTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runTransitionTestCmd(t, tt.cmd(), "https://example.invalid", tt.args...)
			if err == nil {
				t.Fatalf("expected missing --%s error", tt.wantFlag)
			}
			if out != "" {
				t.Errorf("missing flag stdout = %q, want empty", out)
			}
			if !strings.Contains(err.Error(), "required flag") ||
				!strings.Contains(err.Error(), tt.wantFlag) {
				t.Errorf("missing flag error = %q, want required --%s", err, tt.wantFlag)
			}
		})
	}
}
