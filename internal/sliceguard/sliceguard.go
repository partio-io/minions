// Package sliceguard reports what the earlier slices of a sliced run built.
//
// A sliced run commits one empty marker commit after each slice. The package
// partitions the commits of the run's own branch on those markers and
// extracts the package-level Go declarations and the files each slice added,
// so that a later slice can see the work before it, does not rebuild it, and
// does not undo it.
package sliceguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"path"
	"strings"

	"github.com/partio-io/minions/internal/git"
	"github.com/partio-io/minions/internal/slices"
)

// Contribution is one package-level Go declaration that a slice added.
type Contribution struct {
	Identifier string // the declared name
	File       string // the declaring file, relative to the repository root
	Slice      int    // the slice that added it
}

// span is the stretch of the branch that one completed slice committed: the
// commit at its end (its marker), the commit before its start, and the paths
// its commits changed.
type span struct {
	slice int
	end   string
	start string
	files []string
}

// Contributions reports the package-level Go declarations that the slices
// before num added, on the branch ref that grew from base, in the repository
// at dir.
//
// Only commits in base..ref take part, so a declaration base already carried
// is never a contribution. The first slice is compared against the fork
// point of ref from base, not against base's tip: base moves on while a run
// builds, and a declaration base dropped after the fork is not the first
// slice's work. A file the analysis cannot read or parse at a boundary yields
// no contributions and no error.
//
// The result reflects the branch at the last completed marker: a declaration
// that a later completed slice removed is no contribution any more, and one
// that a later completed slice moved is reported under the file that holds
// it there. Otherwise a slice would be told to use, or to restore, work an
// earlier slice already took away.
func Contributions(dir, base, ref string, num int) ([]Contribution, error) {
	commits, err := git.ListCommitsRange(dir, base, ref)
	if err != nil {
		return nil, err
	}
	fork, err := git.MergeBase(dir, base, ref)
	if err != nil {
		return nil, err
	}
	var out []Contribution
	var lastEnd string
	for _, sp := range partition(commits, fork) {
		if sp.slice >= num {
			break
		}
		lastEnd = sp.end
		before := map[string]map[string]string{}
		for _, file := range sp.files {
			if !isSource(file) {
				continue
			}
			pkgDir := path.Dir(file)
			if _, ok := before[pkgDir]; !ok {
				before[pkgDir] = packageDeclarations(dir, sp.start, pkgDir)
			}
			for _, name := range declarations(dir, sp.end, file) {
				if _, ok := before[pkgDir][name]; !ok {
					out = append(out, Contribution{Identifier: name, File: file, Slice: sp.slice})
				}
			}
		}
	}
	return atRevision(dir, lastEnd, out), nil
}

// atRevision keeps the contributions whose package still declares them at
// rev, under the file that declares them there, and drops the rest.
func atRevision(dir, rev string, built []Contribution) []Contribution {
	var out []Contribution
	current := map[string]map[string]string{}
	for _, c := range built {
		pkgDir := path.Dir(c.File)
		if _, ok := current[pkgDir]; !ok {
			current[pkgDir] = packageDeclarations(dir, rev, pkgDir)
		}
		file, ok := current[pkgDir][c.Identifier]
		if !ok {
			continue
		}
		c.File = file
		out = append(out, c)
	}
	return out
}

// partition groups commits, given newest first, into one span per completed
// slice. The k-th marker commit closes slice k, whatever number the marker
// itself carries: markers are counted in order, exactly as ResumePoint counts
// them, so the slice a declaration is attributed to is the slice the run
// resumes after. Commits after the last marker belong to a slice still in
// progress and form no span.
func partition(commits []git.Commit, start string) []span {
	var spans []span
	cur := span{slice: 1, start: start}
	seen := map[string]bool{}
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		if slices.IsMarker(c.Subject) {
			cur.end = c.Hash
			spans = append(spans, cur)
			cur = span{slice: cur.slice + 1, start: c.Hash}
			seen = map[string]bool{}
			continue
		}
		for _, f := range c.Files {
			if !seen[f] {
				seen[f] = true
				cur.files = append(cur.files, f)
			}
		}
	}
	return spans
}

// isSource reports whether file is a Go source file that can carry a
// contribution. Test files cannot: a declaration only a test uses is what
// the guard exists to catch, so a test file declares nothing for it. Neither
// can a file under a directory the reference scan skips, such as testdata or
// vendor: nothing can reference it, so it must never be reported.
func isSource(file string) bool {
	if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
		return false
	}
	for _, seg := range strings.Split(path.Dir(file), "/") {
		if seg != "." && skipDir(seg) {
			return false
		}
	}
	return true
}

// skipDir reports whether a directory holds no Go source the guard reads:
// hidden directories such as .git, vendored code and test fixtures.
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata"
}

// packageDeclarations returns the package-level identifiers that the source
// files directly under pkgDir declare at rev, each with the file that
// declares it. A slice that moves a declaration between files of one
// package, or renames the file, does not add it: the package already
// declared it.
func packageDeclarations(dir, rev, pkgDir string) map[string]string {
	set := map[string]string{}
	files, err := git.ListTreeFiles(dir, rev, pkgDir)
	if err != nil {
		slog.Debug("sliceguard: cannot list package files", "dir", pkgDir, "rev", rev, "err", err)
		return set
	}
	for _, file := range files {
		if !isSource(file) {
			continue
		}
		for _, name := range declarations(dir, rev, file) {
			set[name] = file
		}
	}
	return set
}

// declarations returns the package-level identifiers file declares at rev, in
// source order: functions, types, constants and variables, exported or not.
// Methods, init and the blank identifier are not declarations a later slice
// can use by name, so they are left out. A file that does not exist at rev,
// or does not parse, declares nothing.
func declarations(dir, rev, file string) []string {
	src, err := git.ExecGitDir(dir, "show", rev+":"+file)
	if err != nil {
		return nil
	}
	f, err := parser.ParseFile(token.NewFileSet(), file, src, parser.SkipObjectResolution)
	if err != nil {
		slog.Debug("sliceguard: skipping unparseable file", "file", file, "rev", rev, "err", err)
		return nil
	}
	var names []string
	add := func(name string) {
		if name != "init" && name != "_" {
			names = append(names, name)
		}
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				add(d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					add(spec.Name.Name)
				case *ast.ValueSpec:
					for _, n := range spec.Names {
						add(n.Name)
					}
				}
			}
		}
	}
	return names
}
