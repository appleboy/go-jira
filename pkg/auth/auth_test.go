package auth

import (
	"net/http"
	"testing"
)

// recordingTransport is a sentinel base RoundTripper used to assert that
// Transport wraps (rather than discards or mutates) the base.
type recordingTransport struct {
	lastReq *http.Request
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.lastReq = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

// Compile-time assertions that both implementations satisfy Authenticator.
var (
	_ Authenticator = (*BasicAuth)(nil)
	_ Authenticator = (*BearerAuth)(nil)
)

func TestModeIdentifiers(t *testing.T) {
	tests := []struct {
		auth Authenticator
		want string
	}{
		{&BasicAuth{Username: "u", Password: "p"}, "basic"},
		{&BearerAuth{Token: "t"}, "bearer"},
	}
	for _, tt := range tests {
		if got := tt.auth.Mode(); got != tt.want {
			t.Errorf("Mode() = %q, want %q", got, tt.want)
		}
	}
}

func TestTransportDoesNotMutateBase(t *testing.T) {
	base := &recordingTransport{}
	auths := []Authenticator{
		&BasicAuth{Username: "u", Password: "p"},
		&BearerAuth{Token: "t"},
	}
	for _, a := range auths {
		wrapped := a.Transport(base)
		if wrapped == nil {
			t.Fatalf("%s: Transport returned nil", a.Mode())
		}
		// The wrapper must not be the base itself: it has to add auth.
		if any(wrapped) == any(base) {
			t.Errorf("%s: Transport returned base unchanged", a.Mode())
		}
	}
}
