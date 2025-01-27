package util

import (
	"os"
	"testing"
)

func TestToBool(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "true string",
			input: "true",
			want:  true,
		},
		{
			name:  "TRUE string",
			input: "TRUE",
			want:  true,
		},
		{
			name:  "1 string",
			input: "1",
			want:  true,
		},
		{
			name:  "false string",
			input: "false",
			want:  false,
		},
		{
			name:  "FALSE string",
			input: "FALSE",
			want:  false,
		},
		{
			name:  "0 string",
			input: "0",
			want:  false,
		},
		{
			name:  "random string",
			input: "random",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToBool(tt.input)
			if got != tt.want {
				t.Errorf("ToBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetGlobalValue(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		envVars     map[string]string
		want        string
		shouldClear []string
	}{
		{
			name: "get INPUT_ prefixed value",
			key:  "mykey",
			envVars: map[string]string{
				"INPUT_MYKEY": "input-value",
				"MYKEY":       "regular-value",
			},
			want:        "input-value",
			shouldClear: []string{"INPUT_MYKEY", "MYKEY"},
		},
		{
			name: "fallback to non-prefixed when INPUT_ not set",
			key:  "testkey",
			envVars: map[string]string{
				"TESTKEY": "test-value",
			},
			want:        "test-value",
			shouldClear: []string{"TESTKEY"},
		},
		{
			name:        "empty when no env vars set",
			key:         "missing",
			envVars:     map[string]string{},
			want:        "",
			shouldClear: []string{},
		},
		{
			name: "case insensitive key lookup",
			key:  "MixedCase",
			envVars: map[string]string{
				"INPUT_MIXEDCASE": "mixed-value",
			},
			want:        "mixed-value",
			shouldClear: []string{"INPUT_MIXEDCASE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Clean up environment variables after test
			defer func() {
				for _, k := range tt.shouldClear {
					os.Unsetenv(k)
				}
			}()

			got := GetGlobalValue(tt.key)
			if got != tt.want {
				t.Errorf("GetGlobalValue(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
