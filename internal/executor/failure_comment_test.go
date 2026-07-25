package executor

import (
	"strings"
	"testing"
)

// TestSliceFailureComment_StartsWithMachineMarker pins the stable marker so
// future tooling can identify the runtime's failure comments, following the
// existing <!-- minion:* --> comment-marker convention.
func TestSliceFailureComment_StartsWithMachineMarker(t *testing.T) {
	body := sliceFailureComment(3, 5, "Wire the parser", "minion/implement-implement-42")
	if !strings.HasPrefix(body, "<!-- minion:slice-failure -->\n") {
		t.Errorf("comment does not start with the machine marker:\n%s", body)
	}
}

// TestSliceFailureComment_Content pins the fields the human needs: the failed
// slice (number, title, total), how much of the build the branch holds, and
// the resume instruction — including the nothing-completed first-slice case.
func TestSliceFailureComment_Content(t *testing.T) {
	body := sliceFailureComment(3, 5, "Wire the parser", "minion/implement-implement-42")
	for _, want := range []string{
		"slice 3/5",
		"Wire the parser",
		"`minion/implement-implement-42`",
		"2 of 5",
		"resumes from slice 3",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("comment missing %q:\n%s", want, body)
		}
	}

	first := sliceFailureComment(1, 4, "Bootstrap", "minion/implement-implement-7")
	if !strings.Contains(first, "0 of 4") {
		t.Errorf("first-slice failure does not report zero completed slices:\n%s", first)
	}
}
