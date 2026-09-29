package sliceguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertDeleted runs the deletion scan on repo for the slices before num and
// compares the findings.
func assertDeleted(t *testing.T, repo string, num int, want []Deletion) {
	t.Helper()
	got, err := Deleted(repo, "main", "minion/task-1", num)
	if err != nil {
		t.Fatalf("Deleted: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("Deleted = %+v; want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Deleted[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}
}

// removeDisk deletes files from the working tree of repo without committing:
// the state a slice session leaves behind when it undoes earlier work.
func removeDisk(t *testing.T, repo string, files ...string) {
	t.Helper()
	for _, name := range files {
		if err := os.Remove(filepath.Join(repo, name)); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDeleted_TracerBullet is the headline case: slice one added a Go file
// and a schema, slice two removed the Go file from disk, so the working tree
// no longer holds a file an earlier slice added. The sibling case shows the
// same tree with the file still there: no finding.
func TestDeleted_TracerBullet(t *testing.T) {
	sliceOne := map[string]string{
		"built/slice1.go":   "package built\n\nfunc Slice1() {}\n",
		"assets/schema.sql": "create table t (id int);\n",
	}
	build := func() string {
		return buildRepo(t, map[string]string{"go.mod": goMod}, 2, []step{
			{files: sliceOne},
			{marker: 1},
		})
	}

	t.Run("a deleted earlier file is reported", func(t *testing.T) {
		repo := build()
		removeDisk(t, repo, "built/slice1.go")
		assertDeleted(t, repo, 2, []Deletion{{File: "built/slice1.go", Slice: 1}})
	})

	t.Run("an earlier file still on disk is not reported", func(t *testing.T) {
		repo := build()
		writeDisk(t, repo, map[string]string{"built/slice2.go": "package built\n\nfunc Slice2() { Slice1() }\n"})
		assertDeleted(t, repo, 2, nil)
	})
}

// TestFailureText_Deleted covers the deletion section of the fix session's
// text: each deleted file with the slice that added it, and the one repair
// the guard accepts. Without deletions the section is absent, and the
// abandonment section stays as it was.
func TestFailureText_Deleted(t *testing.T) {
	deleted := []Deletion{
		{File: "built/slice1.go", Slice: 1},
		{File: "assets/schema.sql", Slice: 2},
	}
	text := FailureText(nil, deleted)
	for _, want := range []string{
		"Slice boundary guard",
		"built/slice1.go, added by slice 1",
		"assets/schema.sql, added by slice 2",
		"restore", "Do not delete",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("FailureText lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "duplicate") {
		t.Errorf("FailureText names a duplicate with no abandoned contribution:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Errorf("FailureText does not end on its own line:\n%q", text)
	}

	both := FailureText([]Contribution{{Identifier: "ParseReport", File: "report/report.go", Slice: 1}}, deleted)
	for _, want := range []string{"`ParseReport`", "drop the duplicate", "built/slice1.go, added by slice 1", "restore"} {
		if !strings.Contains(both, want) {
			t.Errorf("FailureText with both kinds lacks %q:\n%s", want, both)
		}
	}
	if FailureText(nil, nil) != "" {
		t.Errorf("FailureText with no findings = %q; want empty", FailureText(nil, nil))
	}
}

// TestDeleted_BaseFileIsNotReported covers the boundary of the rule: only
// files this run's earlier slices added count. A file the base branch
// carried is not reported, whether the current slice deletes it on disk, an
// earlier slice deleted it in a commit, or an earlier slice edited it before
// the current slice deleted it.
func TestDeleted_BaseFileIsNotReported(t *testing.T) {
	base := map[string]string{
		"go.mod":         goMod,
		"legacy/old.go":  "package legacy\n\nfunc Old() {}\n",
		"legacy/keep.go": "package legacy\n\nfunc Keep() {}\n",
	}
	sliceOne := map[string]string{"built/slice1.go": "package built\n\nfunc Slice1() {}\n"}

	t.Run("deleted on disk by the current slice", func(t *testing.T) {
		repo := buildRepo(t, base, 2, []step{{files: sliceOne}, {marker: 1}})
		removeDisk(t, repo, "legacy/old.go")
		assertDeleted(t, repo, 2, nil)
	})
	t.Run("deleted in a commit by an earlier slice", func(t *testing.T) {
		repo := buildRepo(t, base, 2, []step{{files: sliceOne, remove: []string{"legacy/old.go"}}, {marker: 1}})
		assertDeleted(t, repo, 2, nil)
	})
	t.Run("edited by an earlier slice, then deleted on disk", func(t *testing.T) {
		edited := map[string]string{"legacy/old.go": "package legacy\n\nfunc Old() { Keep() }\n"}
		repo := buildRepo(t, base, 2, []step{{files: sliceOne}, {files: edited}, {marker: 1}})
		removeDisk(t, repo, "legacy/old.go")
		assertDeleted(t, repo, 2, nil)
	})
}

// TestDeleted_RenamedFileIsReported covers a rename: the current slice moved
// slice one's file to a new path, so the path the earlier slice added no
// longer exists and is reported, whether or not the new path holds the same
// content.
func TestDeleted_RenamedFileIsReported(t *testing.T) {
	repo := buildRepo(t, map[string]string{"go.mod": goMod}, 2, []step{
		{files: map[string]string{"built/slice1.go": "package built\n\nfunc Slice1() {}\n"}},
		{marker: 1},
	})
	if err := os.Rename(filepath.Join(repo, "built", "slice1.go"), filepath.Join(repo, "built", "one.go")); err != nil {
		t.Fatal(err)
	}
	assertDeleted(t, repo, 2, []Deletion{{File: "built/slice1.go", Slice: 1}})
}

// TestDeleted_SpanEdges pins the partition rules the scan relies on: slice
// one has no earlier slice; a file added and removed inside one slice was
// never that slice's addition; the current slice's own commits form no
// span; and each deleted file names the slice that added it, in slice
// order.
func TestDeleted_SpanEdges(t *testing.T) {
	one := map[string]string{"built/slice1.go": "package built\n\nfunc Slice1() {}\n"}
	two := map[string]string{"assets/schema.sql": "create table t (id int);\n"}

	t.Run("slice one has no earlier slice", func(t *testing.T) {
		repo := buildRepo(t, map[string]string{"go.mod": goMod}, 2, []step{{files: one}})
		removeDisk(t, repo, "built/slice1.go")
		assertDeleted(t, repo, 1, nil)
	})
	t.Run("added and removed inside one slice", func(t *testing.T) {
		repo := buildRepo(t, map[string]string{"go.mod": goMod}, 2, []step{
			{files: map[string]string{"scratch.txt": "tmp"}},
			{files: one, remove: []string{"scratch.txt"}},
			{marker: 1},
		})
		assertDeleted(t, repo, 2, nil)
	})
	t.Run("the current slice's own commits form no span", func(t *testing.T) {
		repo := buildRepo(t, map[string]string{"go.mod": goMod}, 2, []step{
			{files: one},
			{marker: 1},
			{files: two},
		})
		removeDisk(t, repo, "assets/schema.sql")
		assertDeleted(t, repo, 2, nil)
	})
	t.Run("each file names its slice, in slice order", func(t *testing.T) {
		repo := buildRepo(t, map[string]string{"go.mod": goMod}, 3, []step{
			{files: two},
			{marker: 1},
			{files: one},
			{marker: 2},
		})
		removeDisk(t, repo, "built/slice1.go", "assets/schema.sql")
		assertDeleted(t, repo, 3, []Deletion{
			{File: "assets/schema.sql", Slice: 1},
			{File: "built/slice1.go", Slice: 2},
		})
	})
}

// TestDeleted_LaterSliceRemovedFile: a file slice one added and slice two
// deleted in a commit is still reported for slice three. The rule has no
// exception for a deletion the branch already carries: with the guard in
// place slice two could not have passed, so the branch reached this state
// without it, and the tree is still missing an earlier slice's work. A file
// slice two rewrote is present and is not reported.
func TestDeleted_LaterSliceRemovedFile(t *testing.T) {
	one := map[string]string{
		"built/slice1.go":   "package built\n\nfunc Slice1() {}\n",
		"assets/schema.sql": "create table t (id int);\n",
	}
	repo := buildRepo(t, map[string]string{"go.mod": goMod}, 3, []step{
		{files: one},
		{marker: 1},
		{files: map[string]string{"built/slice2.go": "package built\n\nfunc Slice2() {}\n"}, remove: []string{"built/slice1.go"}},
		{files: map[string]string{"assets/schema.sql": "create table t (id int, name text);\n"}},
		{marker: 2},
	})
	assertDeleted(t, repo, 3, []Deletion{{File: "built/slice1.go", Slice: 1}})
}
