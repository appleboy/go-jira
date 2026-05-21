package auth

import (
	"context"
	"github/appleboy/go-jira/pkg/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveOAuthEnv(t *testing.T) {
	srv := newOAuthMockServer(t)
	out := filepath.Join(t.TempDir(), "rotated.txt")

	a, err := Resolve(context.Background(), Config{
		OAuthRefreshToken:       "injected-refresh",
		OAuthRefreshTokenOutput: out,
		OAuthClientID:           "client-abc",
		OAuthClientSecret:       "secret-xyz",
		OAuthBaseURL:            srv.URL,
		OAuthScopes:             []string{"WRITE"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if a.Mode() != "oauth-env" {
		t.Errorf("Mode() = %q, want oauth-env", a.Mode())
	}
	if err := a.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}

	// The initial refresh rotates the token; the new one must be written out.
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read rotation output: %v", err)
	}
	if string(got) != "refresh-1" {
		t.Errorf("rotation output = %q, want refresh-1", got)
	}
}

func TestResolveOAuthEnvRequiresClientSecret(t *testing.T) {
	_, err := Resolve(context.Background(), Config{
		OAuthRefreshToken: "x",
		OAuthClientID:     "client-abc",
		OAuthBaseURL:      "https://jira.example.com",
	})
	if err == nil {
		t.Fatal("expected error when client secret missing in oauth-env mode")
	}
}

func TestResolveOAuthStorage(t *testing.T) {
	srv := newOAuthMockServer(t)
	store := newMemStore()
	key := storage.MakeKey(srv.URL, "client-abc")
	store.m[key] = storedToken(
		srv.URL,
		"access-stored",
		"refresh-stored",
		time.Now().Add(time.Hour),
	)

	a, err := Resolve(context.Background(), Config{
		Store:         store,
		OAuthClientID: "client-abc",
		OAuthBaseURL:  srv.URL,
		OAuthScopes:   []string{"WRITE"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if a.Mode() != "oauth-storage" {
		t.Errorf("Mode() = %q, want oauth-storage", a.Mode())
	}
}

func TestResolveOAuthStorageFallsThroughToBearer(t *testing.T) {
	// Store has no token for this key, so resolution should fall through to the
	// bearer token rather than erroring.
	store := newMemStore()
	a, err := Resolve(context.Background(), Config{
		Store:         store,
		OAuthClientID: "client-abc",
		OAuthBaseURL:  "https://jira.example.com",
		Token:         "pat-123",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if a.Mode() != "bearer" {
		t.Errorf("Mode() = %q, want bearer", a.Mode())
	}
}
