package auth

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		wantMode string
		wantErr  bool
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
			name:    "username without password errors",
			cfg:     Config{Username: "u"},
			wantErr: true,
		},
		{
			name:    "password without username errors",
			cfg:     Config{Password: "p"},
			wantErr: true,
		},
		{
			name:    "nothing configured errors",
			cfg:     Config{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := Resolve(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
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
