package main

import (
	"slices"
	"testing"
)

func TestGetIssueKeys(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		issuePattern string
		want         []string
		wantErr      bool
	}{
		{
			name:         "default pattern",
			ref:          "This is a test ABC-1234 and another DEF-5678",
			issuePattern: "",
			want:         []string{"ABC-1234", "DEF-5678"},
		},
		{
			name:         "custom pattern",
			ref:          "This is a test ABC-1234 and another DEF-5678",
			issuePattern: `([A-Z]{3}-[0-9]{4})`,
			want:         []string{"ABC-1234", "DEF-5678"},
		},
		{
			name:         "no matches",
			ref:          "This is a test with no issues",
			issuePattern: "",
			want:         []string{},
		},
		{
			name:         "duplicate issues",
			ref:          "This is a test ABC-1234 and another ABC-1234",
			issuePattern: "",
			want:         []string{"ABC-1234"},
		},
		{
			name:         "invalid regex pattern",
			ref:          "ABC-1234",
			issuePattern: `([invalid`,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getIssueKeys(tt.ref, tt.issuePattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("getIssueKeys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("getIssueKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}
