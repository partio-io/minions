package sliceguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDisk writes files into the working tree of repo without committing
// them: the state a slice session leaves behind before the guard runs.
func writeDisk(t *testing.T, repo string, files map[string]string) {
	t.Helper()
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

// assertAbandoned runs the scan on repo for the contributions before num and
// compares the findings.
func assertAbandoned(t *testing.T, repo string, num int, want []Contribution) {
	t.Helper()
	built, err := Contributions(repo, "main", "minion/task-1", num)
	if err != nil {
		t.Fatalf("Contributions: %v", err)
	}
	got := Abandoned(repo, built)
	if len(got) != len(want) {
		t.Fatalf("Abandoned = %+v; want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Abandoned[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}
}

const goMod = "module example.com/x\n\ngo 1.26\n"

// TestAbandoned_TracerBullet is the headline case: slice one added
// ParseReport, slice two wrote a duplicate on disk and calls that instead, so
// nothing in the tree references slice one's declaration any more. The
// sibling case shows the same tree with the duplicate replaced by a call to
// slice one's code: no finding.
func TestAbandoned_TracerBullet(t *testing.T) {
	sliceOne := []step{
		{files: map[string]string{
			"report/report.go":      "package report\n\nfunc ParseReport() {}\n",
			"report/report_test.go": "package report\n\nimport \"testing\"\n\nfunc TestParseReport(t *testing.T) { ParseReport() }\n",
		}},
		{marker: 1},
	}
	t.Run("duplicate written, earlier code unreferenced", func(t *testing.T) {
		repo := buildRepo(t, map[string]string{"go.mod": goMod}, 2, sliceOne)
		writeDisk(t, repo, map[string]string{
			"report/parse.go": "package report\n\nfunc Parse() {}\n",
			"cmd/main.go":     "package main\n\nimport \"example.com/x/report\"\n\nfunc main() { report.Parse() }\n",
		})
		assertAbandoned(t, repo, 2, []Contribution{{Identifier: "ParseReport", File: "report/report.go", Slice: 1}})
	})
	t.Run("earlier code called from another package", func(t *testing.T) {
		repo := buildRepo(t, map[string]string{"go.mod": goMod}, 2, sliceOne)
		writeDisk(t, repo, map[string]string{
			"cmd/main.go": "package main\n\nimport \"example.com/x/report\"\n\nfunc main() { report.ParseReport() }\n",
		})
		assertAbandoned(t, repo, 2, nil)
	})
}

// TestAbandoned covers the detection matrix. Each case commits slice one's
// work behind a marker, then writes slice two's state to disk without a
// commit, and asks which of slice one's contributions are now abandoned.
func TestAbandoned(t *testing.T) {
	parseReport := Contribution{Identifier: "ParseReport", File: "report/report.go", Slice: 1}
	tests := []struct {
		name  string
		base  map[string]string
		steps []step
		disk  map[string]string
		num   int // slice under verification; 0 means 2
		want  []Contribution
	}{
		{
			name: "commit references it, disk no longer does",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{
					"report/report.go": "package report\n\nfunc ParseReport() {}\n",
					"cmd/main.go":      "package main\n\nimport \"example.com/x/report\"\n\nfunc main() { report.ParseReport() }\n",
				}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nfunc main() { parse() }\n\nfunc parse() {}\n",
			},
			want: []Contribution{parseReport},
		},
		{
			name: "disk references it, commit never did",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nimport \"example.com/x/report\"\n\nfunc main() { report.ParseReport() }\n",
			},
			want: nil,
		},
		{
			name: "referenced only by its own test",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{
					"report/report.go":      "package report\n\nfunc ParseReport() {}\n",
					"report/report_test.go": "package report\n\nimport \"testing\"\n\nfunc TestParseReport(t *testing.T) { ParseReport() }\n",
				}},
				{marker: 1},
			},
			want: []Contribution{parseReport},
		},
		{
			name: "referenced only by an external test package",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{
					"report/report.go":          "package report\n\nfunc ParseReport() {}\n",
					"report/report_ext_test.go": "package report_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/x/report\"\n)\n\nfunc TestParseReport(t *testing.T) { report.ParseReport() }\n",
				}},
				{marker: 1},
			},
			want: []Contribution{parseReport},
		},
		{
			name: "referenced from another function in the same package",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"report/run.go": "package report\n\nfunc Run() { ParseReport() }\n",
			},
			want: nil,
		},
		{
			name: "a type with methods that nothing constructs",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\ntype Report struct{}\n\nfunc (r *Report) Render() {}\n"}},
				{marker: 1},
			},
			want: []Contribution{{Identifier: "Report", File: "report/report.go", Slice: 1}},
		},
		{
			name: "a type used as a field type",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\ntype Report struct{}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nimport \"example.com/x/report\"\n\ntype app struct{ r report.Report }\n\nfunc main() { _ = app{} }\n",
			},
			want: nil,
		},
		{
			name: "main of a main package is never abandoned",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"cmd/main.go": "package main\n\nfunc main() {}\n"}},
				{marker: 1},
			},
			want: nil,
		},
		{
			name: "a repository without go.mod matches the import suffix",
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nimport \"example.com/x/report\"\n\nfunc main() { report.ParseReport() }\n",
			},
			want: nil,
		},
		{
			name: "a package whose name differs from its directory",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"go-report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nimport \"example.com/x/go-report\"\n\nfunc main() { report.ParseReport() }\n",
			},
			want: nil,
		},
		{
			name: "referenced through a dot import",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nimport . \"example.com/x/report\"\n\nfunc main() { ParseReport() }\n",
			},
			want: nil,
		},
		{
			name: "a testdata fixture is never a contribution",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{
					"a/testdata/fix.go": "package fix\n\nfunc Hello() {}\n",
					"a/a.go":            "package a\n\nfunc A() {}\n",
				}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nimport \"example.com/x/a\"\n\nfunc main() { a.A() }\n",
			},
			want: nil,
		},
		{
			name: "same name in another package is not a reference",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"parse/parse.go": "package parse\n\nfunc ParseReport() {}\n",
				"cmd/main.go":    "package main\n\nimport \"example.com/x/parse\"\n\nfunc main() { parse.ParseReport() }\n",
			},
			want: []Contribution{parseReport},
		},
		{
			// The build check already fails on the broken file and tells the
			// fix session what to do; a finding here would tell it to drop
			// a duplicate that may not exist.
			name: "an unparseable file on disk hides the references, so no finding",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
				{marker: 1},
			},
			disk: map[string]string{
				"cmd/main.go": "package main\n\nfunc (\n",
			},
			want: nil,
		},
		{
			name: "referenced through a nested module's import path",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{
					"tools/go.mod":           "module example.com/tools\n\ngo 1.26\n",
					"tools/report/report.go": "package report\n\nfunc ParseReport() {}\n",
				}},
				{marker: 1},
			},
			disk: map[string]string{
				"tools/cmd/main.go": "package main\n\nimport \"example.com/tools/report\"\n\nfunc main() { report.ParseReport() }\n",
			},
			want: nil,
		},
		{
			name: "a nested module's declaration with no reference is still found",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{
					"tools/go.mod":           "module example.com/tools\n\ngo 1.26\n",
					"tools/report/report.go": "package report\n\nfunc ParseReport() {}\n",
				}},
				{marker: 1},
			},
			disk: map[string]string{
				"tools/cmd/main.go": "package main\n\nimport \"example.com/x/report\"\n\nfunc main() { report.ParseReport() }\n",
				"report/report.go":  "package report\n\nfunc ParseReport() {}\n",
			},
			want: []Contribution{{Identifier: "ParseReport", File: "tools/report/report.go", Slice: 1}},
		},
		{
			name: "slice one has nothing to abandon",
			base: map[string]string{"go.mod": goMod},
			steps: []step{
				{files: map[string]string{"report/report.go": "package report\n\nfunc ParseReport() {}\n"}},
			},
			num:  1,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := buildRepo(t, tt.base, 2, tt.steps)
			writeDisk(t, repo, tt.disk)
			num := tt.num
			if num == 0 {
				num = 2
			}
			assertAbandoned(t, repo, num, tt.want)
		})
	}
}

// TestFailureText: the text that scopes the fix session names each abandoned
// identifier, its file and the slice that added it, and fixes the repair
// direction: call the earlier code, drop the duplicate, delete nothing.
func TestFailureText(t *testing.T) {
	text := FailureText([]Contribution{
		{Identifier: "ParseReport", File: "report/report.go", Slice: 1},
		{Identifier: "Render", File: "api/report/render.go", Slice: 2},
	})
	for _, want := range []string{
		"`ParseReport`", "report/report.go", "slice 1",
		"`Render`", "api/report/render.go", "slice 2",
		"call the earlier code", "drop the duplicate", "Do not delete",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("FailureText lacks %q:\n%s", want, text)
		}
	}
	if !strings.HasSuffix(text, "\n") {
		t.Errorf("FailureText does not end on its own line:\n%q", text)
	}
}
