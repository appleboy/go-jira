package util

import "testing"

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
