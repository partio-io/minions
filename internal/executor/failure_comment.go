package executor

import (
	"fmt"
	"log/slog"
	"strings"
)

// sliceFailureMarker is the stable machine marker opening every slice-failure
// comment, following the <!-- minion:* --> convention of the research
// pipeline's comments, so future tooling can identify them.
const sliceFailureMarker = "<!-- minion:slice-failure -->"

// sliceFailureComment composes the one comment the runtime ever posts on an
// issue: where the build stopped and what to do next. Written for the human
// reading the issue — no log dumps; the workflow's generic failed-label step
// stays the backstop for everything else.
func sliceFailureComment(num, total int, title, branch string) string {
	var b strings.Builder
	b.WriteString(sliceFailureMarker + "\n\n")
	fmt.Fprintf(&b, "Minion build failed at slice %d/%d — %s.\n\n", num, total, title)
	fmt.Fprintf(&b, "Completed slices pushed on branch `%s`: %d of %d.\n", branch, num-1, total)
	fmt.Fprintf(&b, "Re-triggering the build resumes from slice %d.\n", num)
	return b.String()
}

// postSliceFailureComment posts the failure comment on the triggering issue.
// Best-effort by design: a posting failure is logged and never masks the
// build failure the caller is about to report. Runs without an issue (PR-
// triggered) have nowhere to post and stay silent.
func postSliceFailureComment(opts Opts, num, total int, title, branch string) {
	if opts.IssueRef == "" || opts.Project == nil {
		return
	}
	repo := opts.Project.PrincipalFullName()
	if err := postIssueComment(repo, opts.IssueRef, sliceFailureComment(num, total, title, branch)); err != nil {
		slog.Warn("failed to post slice-failure comment", "repo", repo, "issue", opts.IssueRef, "error", err)
	}
}
