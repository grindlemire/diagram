package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// diagramPath is the import path whose Diagram type marks a diagram function.
const diagramPath = "github.com/grindlemire/diagram"

// A diagramFunc is a discovered diagram function.
type diagramFunc struct {
	Func string // Go function name, e.g. PaymentsFlow
	Name string // output name, e.g. payments-flow
	Pos  string // file:line of the declaration, file relative to the package directory
}

// discovery is the result of scanning a package directory.
type discovery struct {
	Package  string // package name
	Funcs    []diagramFunc
	Warnings []string
}

// discover parses the non-test Go files in dir that match the default build
// context and returns every exported top-level function with the signature
// func() *diagram.Diagram, whatever name each file imports the diagram
// package under. Functions are sorted by output name. Two functions with the
// same output name are an error.
func discover(dir string) (*discovery, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	out := &discovery{}
	var errs []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if ok, err := build.Default.MatchFile(dir, name); err != nil || !ok {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			errs = append(errs, relPositions(err.Error(), dir))
			continue
		}
		if out.Package == "" {
			out.Package = f.Name.Name
		}
		funcs, warn := scanFile(fset, f, name)
		out.Funcs = append(out.Funcs, funcs...)
		out.Warnings = append(out.Warnings, warn...)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	sort.Slice(out.Funcs, func(i, j int) bool { return out.Funcs[i].Name < out.Funcs[j].Name })
	for i := 1; i < len(out.Funcs); i++ {
		a, b := out.Funcs[i-1], out.Funcs[i]
		if a.Name == b.Name {
			return nil, fmt.Errorf("%s: %s and %s (%s) both produce %q; rename one", b.Pos, b.Func, a.Func, a.Pos, a.Name)
		}
	}
	return out, nil
}

// relPositions shortens absolute file names under dir in parser errors.
func relPositions(msg, dir string) string {
	return strings.ReplaceAll(msg, dir+string(filepath.Separator), "")
}

// scanFile returns the diagram functions declared in f. file is the name
// used in positions.
func scanFile(fset *token.FileSet, f *ast.File, file string) ([]diagramFunc, []string) {
	var names []string // local names of the diagram package in this file
	var warns []string
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != diagramPath {
			continue
		}
		switch {
		case imp.Name == nil:
			names = append(names, "diagram")
		case imp.Name.Name == ".":
			warns = append(warns, fmt.Sprintf("%s:%d: %s is dot-imported; diagram functions in this file are not discovered (import it by name)",
				file, fset.Position(imp.Pos()).Line, diagramPath))
		case imp.Name.Name != "_":
			names = append(names, imp.Name.Name)
		}
	}
	if len(names) == 0 {
		return nil, warns
	}
	var out []diagramFunc
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !fn.Name.IsExported() || !isDiagramSig(fn.Type, names) {
			continue
		}
		out = append(out, diagramFunc{
			Func: fn.Name.Name,
			Name: kebab(fn.Name.Name),
			Pos:  fmt.Sprintf("%s:%d", file, fset.Position(fn.Pos()).Line),
		})
	}
	return out, warns
}

// isDiagramSig reports whether t is func() *X.Diagram for one of the given
// package names X.
func isDiagramSig(t *ast.FuncType, pkgNames []string) bool {
	if t.TypeParams != nil && len(t.TypeParams.List) > 0 {
		return false
	}
	if t.Params != nil && len(t.Params.List) > 0 {
		return false
	}
	if t.Results == nil || len(t.Results.List) != 1 || len(t.Results.List[0].Names) > 1 {
		return false
	}
	star, ok := t.Results.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Diagram" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	for _, n := range pkgNames {
		if x.Name == n {
			return true
		}
	}
	return false
}

// kebab converts a Go identifier to kebab case: PaymentsFlow becomes
// payments-flow, HTTPServer http-server and OrdersV2 orders-v2. Digits stay
// with the word before them, and underscores become hyphens.
func kebab(s string) string {
	r := []rune(s)
	var b strings.Builder
	for i, c := range r {
		if c == '_' {
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
			continue
		}
		if i > 0 && b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			prev := r[i-1]
			upper := unicode.IsUpper(c)
			switch {
			case upper && (unicode.IsLower(prev) || unicode.IsDigit(prev)):
				// fooBar, v2Api
				b.WriteByte('-')
			case upper && unicode.IsUpper(prev) && i+1 < len(r) && unicode.IsLower(r[i+1]):
				// HTTPServer: the S starts a new word
				b.WriteByte('-')
			}
		}
		b.WriteRune(unicode.ToLower(c))
	}
	return strings.TrimSuffix(b.String(), "-")
}
