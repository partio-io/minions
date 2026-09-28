package sliceguard

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/partio-io/minions/internal/slices"
)

// Abandoned reports the contributions that no Go source file in the working
// tree at dir references any more, at the end of slice num of plan. The scan
// reads the files on disk, not a commit, so a duplicate a session wrote but
// did not commit is seen.
//
// A contribution that a slice after num names in the plan is exempt: that
// later slice is the one that will use it, so it is not abandoned yet. The
// exemption lifts once no later slice names it, so the last slice still
// catches a contribution nothing consumed. The guard asks the plan whether a
// later slice names the identifier; it does not read the plan text itself.
//
// A reference is a use of the identifier outside its own declaration: in the
// declaring package, a plain identifier; from another package, a selector on
// an import of the declaring package, resolved through the module path in
// go.mod. Test files do not count, so a declaration only its own test calls
// is abandoned. Neither does a method receiver, so a type with methods that
// nothing constructs is abandoned. The main function of a main package is
// never abandoned: the Go toolchain is its caller.
//
// Imports resolve through every go.mod in the tree, so a nested module's
// packages are found under the nested module's path. Without any go.mod,
// cross-package references are matched on the suffix of the import path.
//
// A file the scan cannot read or parse hides its references, so the scan
// reports nothing at all: the build check fails on that file and tells the
// fix session what to repair, and a finding here would send it after a
// duplicate that may not exist. The guard errs toward silence.
func Abandoned(dir string, built []Contribution, plan *slices.Plan, num int) []Contribution {
	if len(built) == 0 {
		return nil
	}
	refs := scanReferences(dir)
	if refs.partial {
		return nil
	}
	var out []Contribution
	for _, c := range built {
		pkgDir := path.Dir(c.File)
		if c.Identifier == "main" && refs.mainPkgs[pkgDir] {
			continue
		}
		if plan.NamedAfter(num, c.Identifier) {
			continue
		}
		if !refs.uses(pkgDir, c.Identifier) {
			out = append(out, c)
		}
	}
	return out
}

// references is the set of package-level identifiers the working tree uses,
// keyed by the directory, relative to the repository root, of the package
// that declares them.
type references struct {
	modules  []module                   // every go.mod in the tree, longest path first
	partial  bool                       // a source file could not be read or parsed
	used     map[string]map[string]bool // pkgDir -> identifier -> referenced
	mainPkgs map[string]bool            // pkgDir -> declares package main
	pkgNames map[string]string          // pkgDir -> package clause name
}

// module is one go.mod in the tree: its module path and the directory,
// relative to the repository root, that holds it.
type module struct {
	path string
	dir  string
}

// parsedFile is one source file of the working tree, with the directory of
// its package relative to the repository root.
type parsedFile struct {
	file   *ast.File
	pkgDir string
}

func (r references) uses(pkgDir, name string) bool {
	return r.used[pkgDir][name]
}

func (r references) mark(pkgDir, name string) {
	if r.used[pkgDir] == nil {
		r.used[pkgDir] = map[string]bool{}
	}
	r.used[pkgDir][name] = true
}

// packageDir maps an import path to the repository directory of the package
// it names, or "" for a package outside the repository. The import path is
// matched against the module with the longest path that prefixes it, and
// the directory is that module's directory joined with the rest of the
// path. Without any go.mod, the import path itself is kept and matched on
// its suffix by foldSuffixes.
func (r references) packageDir(importPath string) string {
	if len(r.modules) == 0 {
		return importPath
	}
	for _, m := range r.modules {
		if importPath == m.path {
			return m.dir
		}
		if rest, ok := strings.CutPrefix(importPath, m.path+"/"); ok {
			return path.Join(m.dir, rest)
		}
	}
	return ""
}

// scanReferences parses every non-test Go source file under dir, skipping
// the git directory, vendored code and testdata, and records every
// identifier use it finds against the package directory the use resolves to.
// All files are parsed before any is walked, so an import is resolved
// through the package clause of the package it names, not through the last
// segment of its path: the two differ for a directory such as go-report
// that declares package report.
func scanReferences(dir string) references {
	refs := &references{
		used:     map[string]map[string]bool{},
		mainPkgs: map[string]bool{},
		pkgNames: map[string]string{},
	}
	var files []parsedFile
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// A directory the walk cannot read hides its references.
			slog.Debug("sliceguard: cannot walk path", "path", p, "err", err)
			refs.partial = true
			return nil
		}
		if d.IsDir() {
			if p != dir && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.Name() == "go.mod" {
			if mp := modulePath(p); mp != "" {
				refs.modules = append(refs.modules, module{path: mp, dir: path.Dir(rel)})
			}
			return nil
		}
		if !isSource(rel) {
			return nil
		}
		src, readErr := os.ReadFile(p)
		if readErr != nil {
			slog.Debug("sliceguard: cannot read file", "file", rel, "err", readErr)
			refs.partial = true
			return nil
		}
		f, parseErr := parser.ParseFile(token.NewFileSet(), rel, src, parser.SkipObjectResolution)
		if parseErr != nil {
			slog.Debug("sliceguard: skipping unparseable file", "file", rel, "err", parseErr)
			refs.partial = true
			return nil
		}
		pkgDir := path.Dir(rel)
		// A directory may also hold a build-ignored package main file, such
		// as a generator. The importable package is the other one, so main
		// names the directory only when no other package clause is seen.
		if name, ok := refs.pkgNames[pkgDir]; !ok || name == "main" {
			refs.pkgNames[pkgDir] = f.Name.Name
		}
		files = append(files, parsedFile{file: f, pkgDir: pkgDir})
		return nil
	})
	sort.SliceStable(refs.modules, func(i, j int) bool {
		return len(refs.modules[i].path) > len(refs.modules[j].path)
	})
	for _, pf := range files {
		refs.collect(pf.file, pf.pkgDir)
	}
	if len(refs.modules) == 0 {
		refs.foldSuffixes()
	}
	return *refs
}

// modulePath returns the module path declared in the go.mod at file, or "".
func modulePath(file string) string {
	src, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}

// foldSuffixes, used only without a go.mod, copies every reference recorded
// under a full import path onto each repository directory the path could
// name: its own trailing segments. "example.com/x/report" then also counts
// for "x/report" and "report".
func (r references) foldSuffixes() {
	for key, names := range r.used {
		parts := strings.Split(key, "/")
		for i := 1; i < len(parts); i++ {
			suffix := strings.Join(parts[i:], "/")
			for name := range names {
				r.mark(suffix, name)
			}
		}
	}
}

// collect records the identifier uses in f, whose package lives at pkgDir.
//
// Every identifier counts as a use except the names a declaration
// introduces and the type of a method receiver. A selector on an imported
// package name is a use in that package; any other selector names a field
// or method, and only its operand is inspected. A bare identifier is a use
// in the file's own package and in every package the file dot-imports.
// Names that shadow a package-level identifier, such as a struct field or a
// local variable of the same name, also count: the scan errs toward
// silence, never toward a finding it cannot back.
func (r references) collect(f *ast.File, pkgDir string) {
	if f.Name.Name == "main" {
		r.mainPkgs[pkgDir] = true
	}
	imports := map[string]string{}
	var dotDirs []string
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		alias := r.packageName(p)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		switch alias {
		case "_":
			continue
		case ".":
			if dir := r.packageDir(p); dir != "" {
				dotDirs = append(dotDirs, dir)
			}
			continue
		}
		imports[alias] = p
	}
	skip := map[*ast.Ident]bool{}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			skip[d.Name] = true
			if d.Recv != nil {
				ast.Inspect(d.Recv, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						skip[id] = true
					}
					return true
				})
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					skip[spec.Name] = true
				case *ast.ValueSpec:
					for _, n := range spec.Names {
						skip[n] = true
					}
				}
			}
		}
	}
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ImportSpec:
			return false
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok {
				if p, ok := imports[x.Name]; ok {
					if dir := r.packageDir(p); dir != "" {
						r.mark(dir, n.Sel.Name)
					}
					return false
				}
			}
			ast.Inspect(n.X, walk)
			return false
		case *ast.Ident:
			if !skip[n] {
				r.mark(pkgDir, n.Name)
				for _, dir := range dotDirs {
					r.mark(dir, n.Name)
				}
			}
		}
		return true
	}
	ast.Inspect(f, walk)
}

// packageName returns the name a file uses for an import of importPath when
// it gives no explicit alias: the package clause of the package at that
// path when it is in the repository, else the last segment of the path.
func (r references) packageName(importPath string) string {
	if dir := r.packageDir(importPath); dir != "" {
		if name, ok := r.pkgNames[dir]; ok {
			return name
		}
	}
	return path.Base(importPath)
}

// FailureText is the check output for a set of abandoned contributions. It is
// the whole prompt context the fix session gets, so it names each identifier,
// its file and the slice that added it, and states the one repair the guard
// accepts: the current slice calls the earlier code and drops what it wrote
// instead. Deleting the earlier code is the outcome the guard exists to
// prevent, and the text says so. The text ends on its own line.
func FailureText(abandoned []Contribution) string {
	if len(abandoned) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Slice boundary guard: an earlier slice built code that nothing references any more.\n")
	for _, c := range abandoned {
		fmt.Fprintf(&b, "- `%s` in %s, added by slice %d\n", c.Identifier, c.File, c.Slice)
	}
	b.WriteString("This slice wrote a duplicate of that work instead of using it. ")
	b.WriteString("Repair: call the earlier code from this slice's code, and drop the duplicate this slice wrote. ")
	b.WriteString("Do not delete or rename the earlier code.\n")
	return b.String()
}
