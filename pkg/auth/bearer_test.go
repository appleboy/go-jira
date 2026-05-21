package auth

import (
	"context"
	"net/http"
	"testing"
)

func TestBearerAuthValidate(t *testing.T) {
	if err := (&BearerAuth{Token: "t"}).Validate(); err != nil {
		t.Errorf("Validate() with token returned error: %v", err)
	}
	if err := (&BearerAuth{}).Validate(); err == nil {
		t.Error("Validate() without token should error")
	}
}

func TestBearerAuthTransportSetsHeader(t *testing.T) {
	base := &recordingTransport{}
	a := &BearerAuth{Token: "pat-123"}

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
	if got := base.lastReq.Header.Get("Authorization"); got != "Bearer pat-123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer pat-123")
	}
}
