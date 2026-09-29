package sliceguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partio-io/minions/internal/slices"
)

// gitT runs a git command in dir and fails the test on error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// step is one commit on the run's branch: files to write, then either a
// work commit or, when marker is set, the empty marker commit of that slice.
type step struct {
	files  map[string]string
	remove []string
	marker int
}

// buildRepo creates a repo whose main holds baseFiles in one commit, then
// branches minion/task-1 and plays steps on it. It returns the repo path.
func buildRepo(t *testing.T, baseFiles map[string]string, total int, steps []step) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	gitT(t, filepath.Dir(repo), "init", "-q", "-b", "main", repo)
	gitT(t, repo, "config", "user.name", "t")
	gitT(t, repo, "config", "user.email", "t@t")
	write := func(files map[string]string) {
		for name, src := range files {
			path := filepath.Join(repo, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(map[string]string{"README.md": "hello"})
	write(baseFiles)
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "initial")
	gitT(t, repo, "checkout", "-q", "-b", "minion/task-1")
	for _, st := range steps {
		if st.marker > 0 {
			gitT(t, repo, "commit", "-q", "--allow-empty", "-m", slices.MarkerSubject(st.marker, total))
			continue
		}
		write(st.files)
		for _, name := range st.remove {
			gitT(t, repo, "rm", "-q", name)
		}
		gitT(t, repo, "add", "-A")
		gitT(t, repo, "commit", "-q", "-m", "work")
	}
	return repo
}

func TestContributions(t *testing.T) {
	const kinds = `package report

func ParseReport() {}

func helper() {}

type Report struct{}

type row int

const MaxRows = 10

const minRows = 1

var DefaultName = "x"

var (
	a, b int
)

func (Report) Render() {}

func init() {}

var _ = a
`
	tests := []struct {
		name  string
		base  map[string]string
		total int
		steps []step
		num   int
		want  []Contribution
	}{
		{
			name:  "one earlier slice names its identifier, file and slice",
			total: 2,
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			num:  2,
			want: []Contribution{{Identifier: "ParseReport", File: "report/report.go", Slice: 1}},
		},
		{
			name:  "functions, types, constants and variables, exported or not; not methods, init or blank",
			total: 2,
			steps: []step{
				{files: map[string]string{"report/report.go": kinds}},
				{marker: 1},
			},
			num: 2,
			want: []Contribution{
				{Identifier: "ParseReport", File: "report/report.go", Slice: 1},
				{Identifier: "helper", File: "report/report.go", Slice: 1},
				{Identifier: "Report", File: "report/report.go", Slice: 1},
				{Identifier: "row", File: "report/report.go", Slice: 1},
				{Identifier: "MaxRows", File: "report/report.go", Slice: 1},
				{Identifier: "minRows", File: "report/report.go", Slice: 1},
				{Identifier: "DefaultName", File: "report/report.go", Slice: 1},
				{Identifier: "a", File: "report/report.go", Slice: 1},
				{Identifier: "b", File: "report/report.go", Slice: 1},
			},
		},
		{
			name:  "markers attribute each declaration to its slice, and slices from num on are not reported",
			total: 3,
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
				{files: map[string]string{"report/render.go": "package report\n\nfunc Render() {}\n"}},
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n\nfunc Validate() {}\n"}},
				{marker: 2},
				{files: map[string]string{"report/later.go": "package report\n\nfunc Later() {}\n"}},
			},
			num: 3,
			want: []Contribution{
				{Identifier: "ParseReport", File: "report/report.go", Slice: 1},
				{Identifier: "Render", File: "report/render.go", Slice: 2},
				{Identifier: "Validate", File: "report/report.go", Slice: 2},
			},
		},
		{
			name:  "only the slices before num are reported",
			total: 3,
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
				{files: map[string]string{"report/render.go": "package report\n\nfunc Render() {}\n"}},
				{marker: 2},
			},
			num:  2,
			want: []Contribution{{Identifier: "ParseReport", File: "report/report.go", Slice: 1}},
		},
		{
			name:  "declarations the base already carried are not a contribution",
			base:  map[string]string{"report/report.go": "package report\n\nfunc Existing() {}\n"},
			total: 2,
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc Existing() {}\n\nfunc Added() {}\n"}},
				{marker: 1},
			},
			num:  2,
			want: []Contribution{{Identifier: "Added", File: "report/report.go", Slice: 1}},
		},
		{
			name:  "a base declaration a slice moves to another file of the package is not a contribution",
			base:  map[string]string{"report/a.go": "package report\n\nfunc Existing() {}\n"},
			total: 2,
			steps: []step{
				{
					files:  map[string]string{"report/b.go": "package report\n\nfunc Existing() {}\n\nfunc Added() {}\n"},
					remove: []string{"report/a.go"},
				},
				{marker: 1},
			},
			num:  2,
			want: []Contribution{{Identifier: "Added", File: "report/b.go", Slice: 1}},
		},
		{
			name:  "an unparseable file and a test file yield nothing",
			total: 2,
			steps: []step{
				{files: map[string]string{
					"report/broken.go":      "package report\n\nfunc (\n",
					"report/report_test.go": "package report\n\nfunc TestX() {}\n",
					"notes.txt":             "not go",
				}},
				{marker: 1},
			},
			num:  2,
			want: nil,
		},
		{
			name:  "a marker with another total still closes its slice, as ResumePoint counts it",
			total: 3,
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			num:  2,
			want: []Contribution{{Identifier: "ParseReport", File: "report/report.go", Slice: 1}},
		},
		{
			name:  "slice one has no earlier slice",
			total: 2,
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
			},
			num:  1,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := buildRepo(t, tt.base, tt.total, tt.steps)
			got, err := Contributions(repo, "main", "minion/task-1", tt.num)
			if err != nil {
				t.Fatalf("Contributions: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Contributions = %+v; want %+v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("Contributions[%d] = %+v; want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestContributions_BaseMovedOnAfterFork: base changes while the run builds.
// A declaration base dropped after the fork is still on the branch and is
// not slice one's work; a declaration base gained after the fork that slice
// one also added is slice one's work.
func TestContributions_BaseMovedOnAfterFork(t *testing.T) {
	repo := buildRepo(t, map[string]string{"report/report.go": "package report\n\nfunc Old() {}\n"}, 2, []step{
		{files: map[string]string{"report/report.go": "package report\n\nfunc Old() {}\n\nfunc Added() {}\n\nfunc Shared() {}\n"}},
		{marker: 1},
	})
	gitT(t, repo, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(repo, "report", "report.go"), []byte("package report\n\nfunc Shared() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "commit", "-q", "-am", "main moves on: drop Old, add Shared")

	got, err := Contributions(repo, "main", "minion/task-1", 2)
	if err != nil {
		t.Fatalf("Contributions: %v", err)
	}
	want := []Contribution{
		{Identifier: "Added", File: "report/report.go", Slice: 1},
		{Identifier: "Shared", File: "report/report.go", Slice: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("Contributions = %+v; want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Contributions[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}
}

func TestContributions_ErrorsOnUnknownRef(t *testing.T) {
	repo := buildRepo(t, nil, 2, nil)
	if _, err := Contributions(repo, "main", "minion/absent", 2); err == nil {
		t.Fatal("Contributions: want error for unknown ref, got nil")
	}
}

// TestContributions_LaterSliceRemovedOrMoved: the result reflects the
// branch at the last completed marker. A declaration slice one added and
// slice two removed is no contribution for slice three, and one slice two
// moved into another file of the package is reported under that file.
func TestContributions_LaterSliceRemovedOrMoved(t *testing.T) {
	repo := buildRepo(t, map[string]string{"go.mod": goMod}, 3, []step{
		{files: map[string]string{"built/slice1.go": "package built\n\nfunc Gone() {}\n\nfunc Moved() {}\n"}},
		{marker: 1},
		{files: map[string]string{"built/slice2.go": "package built\n\nfunc Moved() {}\n\nfunc Slice2() {}\n"}, remove: []string{"built/slice1.go"}},
		{marker: 2},
	})
	got, err := Contributions(repo, "main", "minion/task-1", 3)
	if err != nil {
		t.Fatalf("Contributions: %v", err)
	}
	want := []Contribution{
		{Identifier: "Moved", File: "built/slice2.go", Slice: 1},
		{Identifier: "Slice2", File: "built/slice2.go", Slice: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("Contributions = %+v; want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Contributions[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}
}
