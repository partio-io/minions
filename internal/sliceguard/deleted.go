package sliceguard

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/partio-io/minions/internal/git"
)

// Deletion is one file that an earlier slice added and the working tree no
// longer holds.
type Deletion struct {
	File  string // the path the earlier slice added, relative to the repository root
	Slice int    // the slice that added it
}

// Deleted reports the files that the slices before num added, on the branch
// ref that grew from base, in the repository at dir, and that the working
// tree at dir no longer holds. A file counts as added by a slice when it is
// absent at the slice's start and present at its marker, whatever its kind:
// a schema or a template is an earlier slice's work as much as Go source.
//
// Only commits in base..ref take part, so a file base already carried is
// never reported, whichever slice deletes it. A file a later completed
// slice deleted on the branch is still reported: the tree is missing an
// earlier slice's work, and the branch could only reach that state without
// the guard. A file the current slice renamed is reported under the path
// the earlier slice added, because that path no longer exists. A file the
// tree cannot stat for a reason other than absence is not reported: the
// guard errs toward silence.
//
// The guard judges only a repository whose language it can analyze. When
// the tree the earlier slices left holds no Go source, Deleted reports
// nothing, so a non-Go repository behaves as it did before the guard, as the
// declaration check already makes it.
func Deleted(dir, base, ref string, num int) ([]Deletion, error) {
	commits, err := git.ListCommitsRange(dir, base, ref)
	if err != nil {
		return nil, err
	}
	fork, err := git.MergeBase(dir, base, ref)
	if err != nil {
		return nil, err
	}
	addedBy := map[string]int{}
	var lastEnd string
	for _, sp := range partition(commits, fork) {
		if sp.slice >= num {
			break
		}
		lastEnd = sp.end
		for _, file := range addedFiles(dir, sp.start, sp.end) {
			addedBy[file] = sp.slice
		}
	}
	if len(addedBy) == 0 || !holdsGoSource(dir, lastEnd) {
		return nil, nil
	}
	var out []Deletion
	for file, slice := range addedBy {
		_, statErr := os.Lstat(filepath.Join(dir, filepath.FromSlash(file)))
		if errors.Is(statErr, fs.ErrNotExist) {
			out = append(out, Deletion{File: file, Slice: slice})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slice != out[j].Slice {
			return out[i].Slice < out[j].Slice
		}
		return out[i].File < out[j].File
	})
	return out, nil
}

// addedFiles returns the paths present at end and absent at start, in dir,
// relative to the repository root. Renames are not detected, so a moved
// file counts as added under its new path. A range git cannot diff yields
// no files: an unreadable span hides its additions, and the guard stays
// silent about them.
func addedFiles(dir, start, end string) []string {
	out, err := git.ExecGitDir(dir, "-c", "core.quotePath=false", "diff", "--name-only", "--diff-filter=A", "--no-renames", start, end)
	if err != nil {
		slog.Debug("sliceguard: cannot diff slice span", "start", start, "end", end, "err", err)
		return nil
	}
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// holdsGoSource reports whether the tree at rev holds a Go source file that
// the guard reads. A repository whose tree holds none is one whose language
// the guard cannot analyze. A tree git cannot list counts as holding none,
// so the guard stays silent.
func holdsGoSource(dir, rev string) bool {
	out, err := git.ExecGitDir(dir, "-c", "core.quotePath=false", "ls-tree", "-r", "--name-only", rev)
	if err != nil {
		slog.Debug("sliceguard: cannot list the tree", "rev", rev, "err", err)
		return false
	}
	for _, file := range strings.Split(out, "\n") {
		if isSource(file) {
			return true
		}
	}
	return false
}
