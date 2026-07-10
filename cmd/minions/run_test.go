package main

import "testing"

func TestParseRef(t *testing.T) {
	// The bare-number path depends on the package-global project config; force it
	// nil so these cases are deterministic and restore it afterwards.
	saved := proj
	proj = nil
	defer func() { proj = saved }()

	tests := []struct {
		name       string
		ref        string
		flagName   string
		wantRepo   string
		wantNumber string
		wantErr    bool
	}{
		{"full PR reference", "partio-io/cli#488", "pr", "partio-io/cli", "488", false},
		{"full issue reference", "partio-io/docs#12", "issue", "partio-io/docs", "12", false},
		{"nested org path", "acme/team/repo#7", "pr", "acme/team/repo", "7", false},
		{"bare number without project config", "488", "pr", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, number, err := parseRef(tt.ref, tt.flagName)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseRef(%q, %q) = (%q, %q, nil); want error", tt.ref, tt.flagName, repo, number)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRef(%q, %q) unexpected error: %v", tt.ref, tt.flagName, err)
			}
			if repo != tt.wantRepo || number != tt.wantNumber {
				t.Errorf("parseRef(%q, %q) = (%q, %q); want (%q, %q)", tt.ref, tt.flagName, repo, number, tt.wantRepo, tt.wantNumber)
			}
		})
	}
}
