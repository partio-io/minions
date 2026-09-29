package slices

import (
	"fmt"
	"regexp"
)

// markerRe matches a whole commit subject produced by MarkerSubject. Slice
// work commits use a different prefix ("slice N/T: ..."), so counting full
// matches never over-counts.
var markerRe = regexp.MustCompile(`^minion:slice \d+/\d+$`)

// MarkerSubject returns the commit subject that marks slice num of total as
// complete. The executor commits it empty after a slice's checks pass;
// ResumePoint counts these markers to decide where a re-run continues.
func MarkerSubject(num, total int) string {
	return fmt.Sprintf("minion:slice %d/%d", num, total)
}

// IsMarker reports whether subject is a slice marker commit subject, for any
// slice number and any slice total. ResumePoint and the slice partition both
// use it, so the two counts of a branch's markers cannot disagree.
func IsMarker(subject string) bool {
	return markerRe.MatchString(subject)
}

// ResumePoint reports how many slices a branch has already completed, given
// the branch's commit subjects and the plan's slice count. A run resumes at
// slice completed+1; completed == total means nothing is left to build. More
// markers than the plan has slices means plan and branch disagree — that is
// an error, never a guess.
func ResumePoint(subjects []string, total int) (completed int, err error) {
	for _, s := range subjects {
		if IsMarker(s) {
			completed++
		}
	}
	if completed > total {
		return 0, fmt.Errorf("branch has %d slice markers but the plan has %d slices; plan and branch disagree", completed, total)
	}
	return completed, nil
}
