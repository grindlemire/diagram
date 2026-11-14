package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// A target is the package whose diagram functions are rendered.
type target struct {
	Dir        string // package directory
	ImportPath string
	ModDir     string // root of the package's module
}

// resolve finds pkg (a package pattern such as "." or "./diagrams") with go
// list, run in the current directory.
func resolve(ctx context.Context, pkg string) (*target, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-e", "-f", "{{.Dir}}\t{{.ImportPath}}\t{{with .Module}}{{.Dir}}{{end}}", pkg)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s: %v\n%s", pkg, err, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 1 {
		return nil, fmt.Errorf("%s matches %d packages; give one package", pkg, len(lines))
	}
	f := strings.Split(lines[0], "\t")
	if len(f) != 3 || f[0] == "" {
		return nil, fmt.Errorf("%s: no package directory found", pkg)
	}
	if f[2] == "" {
		return nil, fmt.Errorf("%s is not in a module; diagram needs module mode", pkg)
	}
	if _, err := os.Stat(f[0]); err != nil {
		return nil, fmt.Errorf("%s: %v", pkg, err)
	}
	return &target{Dir: f[0], ImportPath: f[1], ModDir: f[2]}, nil
}

// runnerData feeds runnerTmpl.
type runnerData struct {
	ImportPath string
	Funcs      []diagramFunc
}

// genRunner returns the source of a main package that builds every function
// in funcs from the package at importPath and renders it.
func genRunner(importPath string, funcs []diagramFunc) ([]byte, error) {
	var b bytes.Buffer
	if err := runnerTmpl.Execute(&b, runnerData{ImportPath: importPath, Funcs: funcs}); err != nil {
		return nil, err
	}
	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("generated runner: %v\n%s", err, b.Bytes())
	}
	return src, nil
}

// A report is what the runner writes to its -report file.
type report struct {
	Diagrams []result `json:"diagrams"`
}

// A result is the outcome for one diagram.
type result struct {
	Name   string   `json:"name"`
	Func   string   `json:"func"`
	Title  string   `json:"title"`
	Files  []string `json:"files"`  // absolute paths written
	Errors []string `json:"errors"` // each possibly several lines of file:line: message
}

// runnerDir creates a fresh directory for a runner under the module's
// .diagram directory. Directories that start with a dot are ignored by
// ./... patterns, and being inside the module lets the runner import the
// target package, even an internal one.
func runnerDir(modDir string) (string, error) {
	base := filepath.Join(modDir, ".diagram")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(base, "run-")
}

// removeRunner deletes a runner directory, and .diagram too if nothing else
// is left in it.
func removeRunner(dir string) {
	os.RemoveAll(dir)
	os.Remove(filepath.Dir(dir)) // only succeeds when empty
}

// A run is one go run of a generated runner.
type run struct {
	Report *report // nil if the runner did not get as far as writing it
	Output string  // combined go and program output: compiler errors, prints, panics
}

// Failed reports whether the run, or any of its diagrams, failed.
func (r *run) Failed() bool {
	if r.Report == nil {
		return true
	}
	for _, d := range r.Report.Diagrams {
		if len(d.Errors) > 0 {
			return true
		}
	}
	return false
}

// execRunner writes src as dir/main.go and runs it with go run from the
// target package's directory, so compiler errors and the library's
// file:line positions are relative to it, as they are under go test. The
// runner writes formats into outDir.
func execRunner(ctx context.Context, t *target, dir string, src []byte, outDir string, formats []string) (*run, error) {
	main := filepath.Join(dir, "main.go")
	if err := os.WriteFile(main, src, 0o644); err != nil {
		return nil, err
	}
	reportPath := filepath.Join(dir, "report.json")
	os.Remove(reportPath)
	rel, err := filepath.Rel(t.Dir, main)
	if err != nil {
		rel = main
	}
	cmd := exec.CommandContext(ctx, "go", "run", rel,
		"-out", outDir, "-formats", strings.Join(formats, ","), "-report", reportPath)
	cmd.Dir = t.Dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	r := &run{Output: cleanOutput(out.String())}
	data, rerr := os.ReadFile(reportPath)
	if rerr == nil {
		var rep report
		if jerr := json.Unmarshal(data, &rep); jerr == nil {
			r.Report = &rep
		}
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return nil, fmt.Errorf("go run: %v", err)
	}
	if r.Report == nil && r.Output == "" && err != nil {
		r.Output = err.Error()
	}
	return r, nil
}

// cleanOutput drops go run's trailing "exit status 1" line, which only
// repeats the exit code.
func cleanOutput(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndex(s, "\n"); i >= 0 && strings.HasPrefix(s[i+1:], "exit status ") {
		s = s[:i]
	} else if strings.HasPrefix(s, "exit status ") {
		s = ""
	}
	return s
}

var runnerTmpl = template.Must(template.New("runner").Parse(`// Code generated by github.com/grindlemire/diagram/cmd/diagram. DO NOT EDIT.
// It is deleted when the diagram command finishes.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	diagram "github.com/grindlemire/diagram"
	target {{printf "%q" .ImportPath}}
)

var diagrams = []struct {
	name, fn string
	build    func() *diagram.Diagram
}{
{{- range .Funcs}}
	{ {{printf "%q" .Name}}, {{printf "%q" .Func}}, target.{{.Func}} },
{{- end}}
}

type result struct {
	Name   string   ` + "`json:\"name\"`" + `
	Func   string   ` + "`json:\"func\"`" + `
	Title  string   ` + "`json:\"title\"`" + `
	Files  []string ` + "`json:\"files\"`" + `
	Errors []string ` + "`json:\"errors\"`" + `
}

func main() {
	out := flag.String("out", ".", "output directory")
	formats := flag.String("formats", "svg", "comma-separated formats")
	reportPath := flag.String("report", "", "write a JSON report here")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	results := []result{}
	failed := false
	for _, d := range diagrams {
		r := result{Name: d.name, Func: d.fn, Files: []string{}, Errors: []string{}}
		render(&r, d.build, *out, strings.Split(*formats, ","))
		failed = failed || len(r.Errors) > 0
		results = append(results, r)
	}
	if *reportPath != "" {
		data, _ := json.Marshal(map[string]any{"diagrams": results})
		if err := os.WriteFile(*reportPath, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if failed {
		os.Exit(1)
	}
}

// render builds one diagram, validates it and writes each format. A panic
// in the diagram function is reported as its error.
func render(r *result, build func() *diagram.Diagram, out string, formats []string) {
	defer func() {
		if v := recover(); v != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("panic: %v\n\n%s", v, debug.Stack()))
		}
	}()
	d := build()
	if d == nil {
		r.Errors = append(r.Errors, r.Func+" returned nil")
		return
	}
	r.Title = d.Title()
	if err := d.Validate(); err != nil {
		r.Errors = append(r.Errors, err.Error())
		return
	}
	var msgs []string // one per failed format, prefixed with the format
	same := true
	for _, f := range formats {
		path := filepath.Join(out, r.Name+"."+f)
		if err := d.WriteFile(path); err != nil {
			if len(msgs) > 0 && !strings.HasSuffix(msgs[0], ": "+err.Error()) {
				same = false
			}
			msgs = append(msgs, f+": "+err.Error())
			continue
		}
		r.Files = append(r.Files, path)
	}
	// Layout and routing errors are the same in every format: report them once.
	if len(msgs) == len(formats) && same && len(msgs) > 0 {
		_, msg, _ := strings.Cut(msgs[0], ": ")
		msgs = []string{msg}
	}
	r.Errors = append(r.Errors, msgs...)
}
`))
