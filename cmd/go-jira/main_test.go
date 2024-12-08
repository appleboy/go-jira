package main

import (
	"reflect"
	"testing"
)

func TestGetIssueKeys(t *testing.T) {
	tests := []struct {
		name        string
		ref         string
		issueFormat string
		want        []string
	}{
		{
			name:        "default pattern",
			ref:         "This is a test ABC-1234 and another DEF-5678",
			issueFormat: "",
			want:        []string{"ABC-1234", "DEF-5678"},
		},
		{
			name:        "custom pattern",
			ref:         "This is a test ABC-1234 and another DEF-5678",
			issueFormat: `([A-Z]{3}-[0-9]{4})`,
			want:        []string{"ABC-1234", "DEF-5678"},
		},
		{
			name:        "no matches",
			ref:         "This is a test with no issues",
			issueFormat: "",
			want:        []string{},
		},
		{
			name:        "duplicate issues",
			ref:         "This is a test ABC-1234 and another ABC-1234",
			issueFormat: "",
			want:        []string{"ABC-1234"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIssueKeys(tt.ref, tt.issueFormat)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getIssueKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}
