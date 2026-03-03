package approve

import (
	"testing"
	"time"
)

func TestShouldApprove(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name   string
		issue  Issue
		delay  time.Duration
		wantOK bool
		wantRe string // substring of reason
	}{
		{
			name: "eligible after delay",
			issue: Issue{
				Number:    1,
				CreatedAt: now.Add(-25 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}},
				Body:      "blah <!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  24 * time.Hour,
			wantOK: true,
		},
		{
			name: "too young",
			issue: Issue{
				Number:    2,
				CreatedAt: now.Add(-1 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}},
				Body:      "<!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  24 * time.Hour,
			wantOK: false,
			wantRe: "too young",
		},
		{
			name: "vetoed with do-not-build",
			issue: Issue{
				Number:    3,
				CreatedAt: now.Add(-48 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}, {Name: "do-not-build"}},
				Body:      "<!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  24 * time.Hour,
			wantOK: false,
			wantRe: "do-not-build",
		},
		{
			name: "already approved",
			issue: Issue{
				Number:    4,
				CreatedAt: now.Add(-48 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}, {Name: "minion-approved"}},
				Body:      "<!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  24 * time.Hour,
			wantOK: false,
			wantRe: "minion-approved",
		},
		{
			name: "already executing",
			issue: Issue{
				Number:    5,
				CreatedAt: now.Add(-48 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}, {Name: "minion-executing"}},
				Body:      "<!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  24 * time.Hour,
			wantOK: false,
			wantRe: "minion-executing",
		},
		{
			name: "already done",
			issue: Issue{
				Number:    6,
				CreatedAt: now.Add(-48 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}, {Name: "minion-done"}},
				Body:      "<!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  24 * time.Hour,
			wantOK: false,
			wantRe: "minion-done",
		},
		{
			name: "already failed",
			issue: Issue{
				Number:    7,
				CreatedAt: now.Add(-48 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}, {Name: "minion-failed"}},
				Body:      "<!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  24 * time.Hour,
			wantOK: false,
			wantRe: "minion-failed",
		},
		{
			name: "no embedded task YAML",
			issue: Issue{
				Number:    8,
				CreatedAt: now.Add(-48 * time.Hour),
				Labels:    []Label{{Name: "minion-proposal"}},
				Body:      "just a plain issue body",
			},
			delay:  24 * time.Hour,
			wantOK: false,
			wantRe: "no embedded",
		},
		{
			name: "zero delay approves immediately",
			issue: Issue{
				Number:    9,
				CreatedAt: now.Add(-1 * time.Minute),
				Labels:    []Label{{Name: "minion-proposal"}},
				Body:      "<!-- minion-task\nid: test\nminion-task -->",
			},
			delay:  0,
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := ShouldApprove(tt.issue, tt.delay)
			if ok != tt.wantOK {
				t.Errorf("ShouldApprove() = %v, want %v (reason: %s)", ok, tt.wantOK, reason)
			}
			if !ok && tt.wantRe != "" {
				if !containsSubstring(reason, tt.wantRe) {
					t.Errorf("reason %q does not contain %q", reason, tt.wantRe)
				}
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestHasLabel(t *testing.T) {
	issue := Issue{
		Labels: []Label{{Name: "minion-proposal"}, {Name: "do-not-build"}},
	}

	if !issue.HasLabel("minion-proposal") {
		t.Error("expected HasLabel to find minion-proposal")
	}
	if !issue.HasLabel("do-not-build") {
		t.Error("expected HasLabel to find do-not-build")
	}
	if issue.HasLabel("nonexistent") {
		t.Error("expected HasLabel to not find nonexistent")
	}
}
