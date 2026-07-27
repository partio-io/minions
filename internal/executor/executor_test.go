package executor

import (
	"strings"
	"testing"

	"github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/program"
)

func TestBuildTaskID(t *testing.T) {
	tests := []struct {
		name      string
		progID    string
		agentName string
		issueRef  string
		want      string
	}{
		{
			name:      "no issue keeps prog-agent shape",
			progID:    "implement",
			agentName: "implement",
			issueRef:  "",
			want:      "implement-implement",
		},
		{
			name:      "issue number is appended for uniqueness",
			progID:    "implement",
			agentName: "implement",
			issueRef:  "437",
			want:      "implement-implement-437",
		},
		{
			name:      "different issue yields a different task id",
			progID:    "implement",
			agentName: "implement",
			issueRef:  "511",
			want:      "implement-implement-511",
		},
		{
			name:      "named agent with issue",
			progID:    "research",
			agentName: "publisher",
			issueRef:  "120",
			want:      "research-publisher-120",
		},
		{
			name:      "named agent without issue",
			progID:    "research",
			agentName: "publisher",
			issueRef:  "",
			want:      "research-publisher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildTaskID(tt.progID, tt.agentName, tt.issueRef); got != tt.want {
				t.Errorf("buildTaskID(%q, %q, %q) = %q, want %q",
					tt.progID, tt.agentName, tt.issueRef, got, tt.want)
			}
		})
	}
}

// TestBuildTaskID_DistinctPerIssue is the regression guard for the shared-branch
// bug: two builds of the same program+agent for different issues must not share
// a task id (and therefore must not share a branch / PR).
func TestBuildTaskID_DistinctPerIssue(t *testing.T) {
	a := buildTaskID("implement", "implement", "437")
	b := buildTaskID("implement", "implement", "438")
	if a == b {
		t.Fatalf("expected distinct task ids per issue, both were %q", a)
	}
}

// TestBuildAgentPrompt_ForbidsSessionPRs pins the runtime-boundary directive:
// every session prompt (single-session and slice builds share this builder)
// must tell the agent that pushing and PR creation belong to the runtime.
// On 2026-07-25 two staged builds failed because sessions ran `gh pr create`
// themselves after the issue text mentioned a PR.
func TestBuildAgentPrompt_ForbidsSessionPRs(t *testing.T) {
	prog := &program.Program{
		ID:          "implement",
		Title:       "Implement the issue",
		Description: "Build what the issue asks for.",
		TargetRepos: []string{"api"},
	}
	agent := &program.AgentDef{Name: "implement"}
	pt := context.NewTracker("implement").StartPhase("agent:implement")

	prompt := buildAgentPrompt(prog, agent, "", "", "", t.TempDir(), nil, pt)

	for _, want := range []string{"Never push or open a pull request", "gh pr create"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing runtime-boundary directive %q", want)
		}
	}
}
