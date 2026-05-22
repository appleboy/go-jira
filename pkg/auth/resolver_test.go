package auth

import (
	"context"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		wantMode    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "token wins over basic",
			cfg:      Config{Token: "t", Username: "u", Password: "p"},
			wantMode: "bearer",
		},
		{
			name:     "bearer only",
			cfg:      Config{Token: "t"},
			wantMode: "bearer",
		},
		{
			name:     "basic when no token",
			cfg:      Config{Username: "u", Password: "p"},
			wantMode: "basic",
		},
		{
			name:        "username without password errors",
			cfg:         Config{Username: "u"},
			wantErr:     true,
			errContains: "basic auth requires both",
		},
		{
			name:        "password without username errors",
			cfg:         Config{Password: "p"},
			wantErr:     true,
			errContains: "basic auth requires both",
		},
		{
			name:        "nothing configured errors",
			cfg:         Config{},
			wantErr:     true,
			errContains: "no authentication configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := Resolve(context.Background(), tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if a.Mode() != tt.wantMode {
				t.Errorf("Mode() = %q, want %q", a.Mode(), tt.wantMode)
			}
			if err := a.Validate(); err != nil {
				t.Errorf("resolved authenticator failed Validate(): %v", err)
			}
		})
	}
}
