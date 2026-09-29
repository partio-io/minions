// Package slices parses the per-slice build plan that the research minion
// publishes as an issue comment (marked minion:research-slices).
package slices

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Comment is one GitHub issue comment, passed structurally from the run
// command so plan detection works on raw comment bodies rather than the
// rendered context blob.
type Comment struct {
	Author string
	Body   string
}

// Slice is one entry of a slice plan.
type Slice struct {
	Number             int
	Title              string
	Description        string
	AcceptanceCriteria []string
	ModulesTouched     []string
	OutOfScope         []string
}

// Plan is an ordered slice plan parsed from a plan comment.
type Plan struct {
	Slices []Slice
	Raw    string // the plan comment body, verbatim
}

// Marker lines are matched as whole lines so prose that merely mentions a
// marker (e.g. a design comment discussing minion:research-slices) never
// counts as carrying one. The prd regex requires whitespace or --> right
// after "research" so it cannot match the -slices marker.
var (
	planMarkerRe   = regexp.MustCompile(`(?m)^\s*<!--\s*minion:research-slices(\s[^>]*)?-->\s*$`)
	prdMarkerRe    = regexp.MustCompile(`(?m)^\s*<!--\s*minion:research(\s[^>]*)?-->\s*$`)
	sliceHeadingRe = regexp.MustCompile(`^#{2,4}\s+Slice\s+(\d+)\s*[—–-]+\s*(.+?)\s*$`)
	headingRe      = regexp.MustCompile(`^#{2,6}\s+(.+?)\s*:?\s*$`)
	bulletRe       = regexp.MustCompile(`^\s*[-*]\s+(.+)$`)
	checkboxRe     = regexp.MustCompile(`^\[[ xX]\]\s+`)
)

// sections a slice body is routed into, keyed by normalized heading text.
var sectionByHeading = map[string]string{
	"acceptance criteria": "criteria",
	"modules touched":     "modules",
	"out of scope":        "out-of-scope",
}

// appendBullet collects a list item into dst, stripping any leading
// checkbox. Non-bullet lines are ignored.
func appendBullet(dst *[]string, line string) {
	if m := bulletRe.FindStringSubmatch(line); m != nil {
		*dst = append(*dst, checkboxRe.ReplaceAllString(m[1], ""))
	}
}

// FindPlanComment returns the latest comment carrying the
// minion:research-slices marker; a re-published plan supersedes older ones.
func FindPlanComment(comments []Comment) (Comment, bool) {
	for i := len(comments) - 1; i >= 0; i-- {
		if planMarkerRe.MatchString(comments[i].Body) {
			return comments[i], true
		}
	}
	return Comment{}, false
}

// FindPRDComment returns the body of the latest comment carrying the
// minion:research marker (the published PRD).
func FindPRDComment(comments []Comment) (string, bool) {
	for i := len(comments) - 1; i >= 0; i-- {
		if prdMarkerRe.MatchString(comments[i].Body) {
			return comments[i].Body, true
		}
	}
	return "", false
}

// Parse turns a plan comment body into an ordered Plan. Slice descriptions
// are the prose between the slice heading and its first subsection; bullets
// under Acceptance criteria / Modules touched / Out of scope are collected
// with any leading checkbox stripped. Unknown subsections are ignored.
//
// Validation is strict — a malformed plan is an error, never a silent
// fallback: the marker line must be present, slice numbering must run 1..N
// without gaps, and every slice needs at least one acceptance criterion.
func Parse(body string) (*Plan, error) {
	if !planMarkerRe.MatchString(body) {
		return nil, fmt.Errorf("slice plan: missing <!-- minion:research-slices --> marker line")
	}
	plan := &Plan{Raw: body}
	var cur *Slice
	section := ""
	var desc []string

	finish := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(strings.Join(desc, "\n"))
		plan.Slices = append(plan.Slices, *cur)
		cur, desc = nil, nil
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := sliceHeadingRe.FindStringSubmatch(line); m != nil {
			finish()
			n, _ := strconv.Atoi(m[1])
			cur = &Slice{Number: n, Title: m[2]}
			section = "description"
			continue
		}
		if cur == nil {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			section = sectionByHeading[strings.ToLower(m[1])] // "" for unknown headings
			continue
		}
		switch section {
		case "description":
			desc = append(desc, line)
		case "criteria":
			appendBullet(&cur.AcceptanceCriteria, line)
		case "modules":
			appendBullet(&cur.ModulesTouched, line)
		case "out-of-scope":
			appendBullet(&cur.OutOfScope, line)
		}
	}
	finish()

	if len(plan.Slices) == 0 {
		return nil, fmt.Errorf("slice plan: no slices found after the marker (expected `### Slice <n> — <Title>` sections)")
	}
	for i, s := range plan.Slices {
		if s.Number != i+1 {
			return nil, fmt.Errorf("slice plan: numbering gap: expected slice %d, found slice %d (%q)", i+1, s.Number, s.Title)
		}
		if len(s.AcceptanceCriteria) == 0 {
			return nil, fmt.Errorf("slice plan: slice %d (%q): no acceptance criteria", s.Number, s.Title)
		}
	}
	return plan, nil
}

// NamedAfter reports whether any slice after num names ident in its plan
// text. The plan is prose from the research minion, so the text of a slice
// is its whole section of the comment, from its heading to the next slice
// heading, not only the fields Parse extracts: a mention under a subsection
// Parse ignores still counts. The match is on the identifier as a whole
// word, so Parse does not match ParseReport. A nil plan names nothing.
func (p *Plan) NamedAfter(num int, ident string) bool {
	if p == nil {
		return false
	}
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(ident) + `\b`)
	cur := 0
	for _, line := range strings.Split(p.Raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := sliceHeadingRe.FindStringSubmatch(line); m != nil {
			cur, _ = strconv.Atoi(m[1])
		}
		if cur > num && re.MatchString(line) {
			return true
		}
	}
	return false
}
