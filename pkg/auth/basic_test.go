package auth

import (
	"context"
	"net/http"
	"testing"
)

func TestBasicAuthValidate(t *testing.T) {
	tests := []struct {
		name    string
		auth    BasicAuth
		wantErr bool
	}{
		{"both present", BasicAuth{Username: "u", Password: "p"}, false},
		{"missing password", BasicAuth{Username: "u"}, true},
		{"missing username", BasicAuth{Password: "p"}, true},
		{"both missing", BasicAuth{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.auth.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBasicAuthTransportSetsCredentials(t *testing.T) {
	base := &recordingTransport{}
	a := &BasicAuth{Username: "alice", Password: "s3cret"}

	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"https://jira.example.com/rest/api/2/myself",
		nil,
	)
	resp, err := a.Transport(base).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if base.lastReq == nil {
		t.Fatal("base transport never received the request")
	}
	user, pass, ok := base.lastReq.BasicAuth()
	if !ok {
		t.Fatal("expected Basic Auth header on forwarded request")
	}
	if user != "alice" || pass != "s3cret" {
		t.Errorf("BasicAuth() = %q/%q, want alice/s3cret", user, pass)
	}
}
