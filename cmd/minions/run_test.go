package main

import (
	"strings"
	"testing"
)

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

func comment(login, body string) issueComment {
	var c issueComment
	c.Author.Login = login
	c.Body = body
	c.CreatedAt = "2026-07-13T08:15:36Z"
	return c
}

func TestToSliceComments_PassesAllCommentsThrough(t *testing.T) {
	minimized := comment("bot", "hidden but still passed through")
	minimized.IsMinimized = true
	comments := []issueComment{
		comment("jcleira", "<!-- minion:research-slices -->\n\n### Slice 1 — A"),
		comment("jcleira", "Minion completed. status noise"),
		minimized,
	}

	got := toSliceComments(comments)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 — structural pass-through must not filter", len(got))
	}
	for i, c := range comments {
		if got[i].Author != c.Author.Login || got[i].Body != c.Body {
			t.Errorf("comment %d: got %+v, want author %q body %q", i, got[i], c.Author.Login, c.Body)
		}
	}
	if toSliceComments(nil) != nil {
		t.Errorf("nil input should stay nil")
	}
}

func TestIsNoiseComment(t *testing.T) {
	minimized := comment("jcleira", "outdated design sketch")
	minimized.IsMinimized = true

	tests := []struct {
		name string
		c    issueComment
		want bool
	}{
		{
			name: "research comment is signal",
			c:    comment("jcleira", "<!-- minion:research -->\n\n# Design\n\nUse FindByBranch."),
			want: false,
		},
		{
			name: "plain human discussion is signal",
			c:    comment("jcleira", "Don't do squash detection, it's unreliable."),
			want: false,
		},
		{
			name: "trigger followed by real instructions is signal",
			c:    comment("jcleira", "/minion build\n\nUse the orphan store, not tree hashes."),
			want: false,
		},
		{
			name: "failure status update is noise",
			c:    comment("jcleira", "Minion execution failed. [View logs](https://example.com)"),
			want: true,
		},
		{
			name: "completion status update is noise",
			c:    comment("jcleira", "Minion completed — see PR #515."),
			want: true,
		},
		{
			name: "bare slash trigger is noise",
			c:    comment("jcleira", "/minion build\n"),
			want: true,
		},
		{
			name: "empty comment is noise",
			c:    comment("jcleira", "   \n  "),
			want: true,
		},
		{
			name: "minimized comment is noise",
			c:    minimized,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNoiseComment(tt.c); got != tt.want {
				t.Errorf("isNoiseComment() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRenderIssueContext_IncludesComments is the regression guard for the bug
// this code exists to fix: the agent used to receive title + body only, so every
// research comment was silently dropped and every build ran blind.
func TestRenderIssueContext_IncludesComments(t *testing.T) {
	iv := issueView{
		Title: "Support checkpoint resume",
		Body:  "We need resume to work after a squash merge.",
		Comments: []issueComment{
			comment("jcleira", "Out of Scope: squash-merge auto-detection via --commit — unreliable."),
			comment("jcleira", "Minion execution failed. [View logs](https://example.com)"),
			comment("jcleira", "/minion build"),
		},
	}

	got := renderIssueContext(iv, maxCommentChars)

	for _, want := range []string{
		"# Support checkpoint resume",
		"We need resume to work after a squash merge.",
		"Out of Scope: squash-merge auto-detection",
		"@jcleira — 2026-07-13",
		"2 comments of automated minion status were filtered out",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered context missing %q:\n%s", want, got)
		}
	}
	for _, notWant := range []string{"Minion execution failed", "/minion build"} {
		if strings.Contains(got, notWant) {
			t.Errorf("noise comment %q leaked into context:\n%s", notWant, got)
		}
	}
}

func TestRenderIssueContext_NoComments(t *testing.T) {
	iv := issueView{Title: "Fix the thing", Body: "It is broken."}

	got := renderIssueContext(iv, maxCommentChars)

	if want := "# Fix the thing\n\nIt is broken."; got != want {
		t.Errorf("renderIssueContext() = %q, want %q", got, want)
	}
}

// TestRenderIssueContext_OnlyNoiseComments guards the case where every comment
// is machinery: the output must match having no comments at all, with no empty
// Discussion section left behind.
func TestRenderIssueContext_OnlyNoiseComments(t *testing.T) {
	iv := issueView{
		Title:    "Fix the thing",
		Body:     "It is broken.",
		Comments: []issueComment{comment("jcleira", "/minion build")},
	}

	if got, want := renderIssueContext(iv, maxCommentChars), "# Fix the thing\n\nIt is broken."; got != want {
		t.Errorf("renderIssueContext() = %q, want %q", got, want)
	}
}

// TestRenderIssueContext_TruncationIsDisclosed covers the honesty requirement:
// a thread too large for the budget must say so, because silent truncation
// reads to the agent as "you have everything".
func TestRenderIssueContext_TruncationIsDisclosed(t *testing.T) {
	big := strings.Repeat("x", 400)
	iv := issueView{
		Title: "Big thread",
		Body:  "Body.",
		Comments: []issueComment{
			comment("jcleira", "OLDEST — the design"),
			comment("jcleira", big),
			comment("jcleira", big),
			comment("jcleira", "NEWEST — the course correction"),
		},
	}

	// Enough room for the two short comments and nothing more.
	got := renderIssueContext(iv, 200)

	for _, want := range []string{
		"OLDEST — the design",
		"NEWEST — the course correction",
		"exceeded the context budget",
		"_[2 comments omitted to fit the context budget]_",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered context missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, big) {
		t.Errorf("oversized comment should have been dropped:\n%s", got)
	}
}

func TestSelectComments(t *testing.T) {
	small := comment("jcleira", strings.Repeat("a", 10))
	huge := comment("jcleira", strings.Repeat("a", 10000))

	tests := []struct {
		name     string
		comments []issueComment
		budget   int
		want     []bool
	}{
		{
			name:     "everything fits",
			comments: []issueComment{small, small, small},
			budget:   maxCommentChars,
			want:     []bool{true, true, true},
		},
		{
			name:     "middle is dropped, both ends kept",
			comments: []issueComment{small, huge, small},
			budget:   200,
			want:     []bool{true, false, true},
		},
		{
			name:     "an oversized head does not block the rest",
			comments: []issueComment{huge, small, small},
			budget:   200,
			want:     []bool{false, true, true},
		},
		{
			name:     "nothing fits",
			comments: []issueComment{huge, huge},
			budget:   200,
			want:     []bool{false, false},
		},
		{
			name:     "no comments",
			comments: nil,
			budget:   200,
			want:     []bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keep, dropped := selectComments(tt.comments, tt.budget)
			if len(keep) != len(tt.want) {
				t.Fatalf("selectComments() returned %d flags, want %d", len(keep), len(tt.want))
			}
			wantDropped := 0
			for i := range tt.want {
				if keep[i] != tt.want[i] {
					t.Errorf("keep[%d] = %v, want %v", i, keep[i], tt.want[i])
				}
				if !tt.want[i] {
					wantDropped++
				}
			}
			if dropped != wantDropped {
				t.Errorf("dropped = %d, want %d", dropped, wantDropped)
			}
		})
	}
}

func TestDiscussionNote(t *testing.T) {
	tests := []struct {
		name            string
		total           int
		kept            int
		filtered        int
		dropped         int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "all comments shown", total: 2, kept: 2,
			wantContains:    []string{"This issue has 2 comments.", "The remaining 2 comments follow, oldest first."},
			wantNotContains: []string{"filtered out", "context budget"},
		},
		{
			name: "singular nouns", total: 1, kept: 1,
			wantContains:    []string{"This issue has 1 comment.", "The remaining 1 comment follow"},
			wantNotContains: []string{"1 comments"},
		},
		{
			name: "filtering disclosed", total: 5, kept: 2, filtered: 3,
			wantContains: []string{"3 comments of automated minion status were filtered out."},
		},
		{
			name: "truncation disclosed", total: 9, kept: 2, filtered: 3, dropped: 4,
			wantContains: []string{
				"4 comments exceeded the context budget and were omitted",
				"assume the discussion below is incomplete",
			},
		},
		{
			name: "single dropped comment reads as singular", total: 2, kept: 1, dropped: 1,
			wantContains: []string{"1 comment exceeded the context budget and was omitted"},
		},
		{
			name: "nothing survived the budget", total: 1, kept: 0, dropped: 1,
			wantContains:    []string{"was omitted"},
			wantNotContains: []string{"The remaining"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discussionNote(tt.total, tt.kept, tt.filtered, tt.dropped)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("discussionNote() = %q, want it to contain %q", got, want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("discussionNote() = %q, want it NOT to contain %q", got, notWant)
				}
			}
		})
	}
}
