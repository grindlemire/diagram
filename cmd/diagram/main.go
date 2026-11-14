// Command diagram finds the diagram functions in a Go package and renders
// them, once or live while you edit.
//
// A diagram function is an exported top-level function with the signature
//
//	func Name() *diagram.Diagram
//
// in a non-test file, where diagram is whatever name the file imports
// github.com/grindlemire/diagram under. Its output name is the function name
// in kebab case: PaymentsFlow is written as payments-flow.svg.
//
// Usage:
//
//	diagram list   [pkg]
//	diagram render [pkg] [-o dir] [-f svg,png,html,drawio] [-only name,...]
//	diagram watch  [pkg] [-addr 127.0.0.1:0] [-o dir] [-f formats]
//	diagram help   [command]
//
// pkg defaults to the package in the current directory. The package can't
// be package main, because the command imports it.
//
// To run the functions, diagram generates a small main package in
// .diagram/ at the root of the package's module, runs it with go run and
// deletes it afterwards (watch keeps it until it exits). Add .diagram/ to
// your .gitignore.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

const usage = `diagram finds the diagram functions in a Go package and renders them.

A diagram function is an exported top-level function with the signature
func() *diagram.Diagram. Its output name is the function name in kebab case
(PaymentsFlow is written as payments-flow.svg).

Usage:

	diagram list   [pkg]
	diagram render [pkg] [-o dir] [-f svg,png,html,drawio] [-only name,...]
	diagram watch  [pkg] [-addr 127.0.0.1:0] [-o dir] [-f formats]
	diagram help   [command]

pkg defaults to "." and must not be package main. The command generates a
runner in .diagram/ at the module root while it works; add .diagram/ to
your .gitignore.

Run "diagram help <command>" for a command's flags.
`

var commandHelp = map[string]string{
	"list": `usage: diagram list [pkg]

Prints the output name, function and position of every diagram function in
pkg (default ".").
`,
	"render": `usage: diagram render [pkg] [-o dir] [-f svg,png,html,drawio] [-only name,...]

Renders every diagram function in pkg (default ".") and prints the files it
wrote. Diagrams that fail validation or rendering are reported with their
file:line and don't stop the others; the exit status is 1 if any failed.
`,
	"watch": `usage: diagram watch [pkg] [-addr 127.0.0.1:0] [-o dir] [-f formats]

Serves an index of the diagrams in pkg (default ".") and each one as an
interactive page, and reloads open pages whenever a .go file in the
package, or go.mod, changes. Build and validation errors are shown in the
page until they are fixed. With -o, also writes the files (-f, default
svg) after every run.
`,
}

// runCLI runs the command line args and returns the exit status.
func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "help", "-h", "-help", "--help":
		if len(rest) > 0 {
			if h, ok := commandHelp[rest[0]]; ok {
				fmt.Fprint(stdout, h)
				return 0
			}
			fmt.Fprintf(stderr, "diagram: unknown command %q\n", rest[0])
			return 2
		}
		fmt.Fprint(stdout, usage)
		return 0
	case "list":
		err = cmdList(ctx, rest, stdout, stderr)
	case "render":
		err = cmdRender(ctx, rest, stdout, stderr)
	case "watch":
		err = cmdWatch(ctx, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "diagram: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		return 2
	case errors.Is(err, errFailed):
		return 1 // already reported
	default:
		fmt.Fprintf(stderr, "diagram %s: %v\n", cmd, err)
		return 1
	}
}

var (
	errUsage  = errors.New("usage")
	errFailed = errors.New("failed")
)

// parseArgs parses flags that may come before or after the one optional
// package argument, and returns the package (default ".").
func parseArgs(fs *flag.FlagSet, args []string, stderr io.Writer) (string, error) {
	fs.SetOutput(stderr)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return "", err
			}
			return "", errUsage
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	switch len(pos) {
	case 0:
		return ".", nil
	case 1:
		return pos[0], nil
	}
	fmt.Fprintf(stderr, "diagram %s: want at most one package, got %s\n", fs.Name(), strings.Join(pos, " "))
	return "", errUsage
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), commandHelp[name])
		if hasFlags(fs) {
			fmt.Fprintln(fs.Output(), "\nFlags:")
			fs.PrintDefaults()
		}
	}
	return fs
}

func hasFlags(fs *flag.FlagSet) bool {
	n := 0
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n > 0
}

// pkgInfo is a resolved target package and its diagram functions.
type pkgInfo struct {
	*target
	*discovery
}

// load resolves pkg and discovers its diagram functions, printing
// discovery warnings to stderr.
func load(ctx context.Context, pkg string, stderr io.Writer) (*pkgInfo, error) {
	t, err := resolve(ctx, pkg)
	if err != nil {
		return nil, err
	}
	d, err := discover(t.Dir)
	if err != nil {
		return nil, err
	}
	for _, w := range d.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	if d.Package == "main" {
		return nil, fmt.Errorf("%s is package main, which can't be imported; move the diagram functions to another package", pkg)
	}
	return &pkgInfo{t, d}, nil
}

func cmdList(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("list")
	pkg, err := parseArgs(fs, args, stderr)
	if err != nil {
		return err
	}
	p, err := load(ctx, pkg, stderr)
	if err != nil {
		return err
	}
	wd, _ := os.Getwd()
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, f := range p.Funcs {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Name, f.Func, rebase(f.Pos, p.Dir, wd))
	}
	tw.Flush()
	if len(p.Funcs) == 0 {
		fmt.Fprintf(stderr, "no diagram functions (func() *diagram.Diagram) in %s\n", pkg)
	}
	return nil
}

// formatList is a -f flag value.
type formatList []string

var allFormats = []string{"svg", "png", "html", "drawio"}

func (f *formatList) String() string { return strings.Join(*f, ",") }
func (f *formatList) Set(s string) error {
	var out []string
	for v := range strings.SplitSeq(s, ",") {
		v = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v), "."))
		if v == "" {
			continue
		}
		if !slices.Contains(allFormats, v) {
			return fmt.Errorf("unknown format %q (want %s)", v, strings.Join(allFormats, ", "))
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return errors.New("no formats")
	}
	*f = out
	return nil
}

// selectFuncs returns the functions named in only (output names or Go
// function names), or all of them if only is empty.
func selectFuncs(funcs []diagramFunc, only string) ([]diagramFunc, error) {
	if only == "" {
		return funcs, nil
	}
	var out []diagramFunc
	for n := range strings.SplitSeq(only, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		i := slices.IndexFunc(funcs, func(f diagramFunc) bool { return f.Name == n || f.Func == n })
		if i < 0 {
			var names []string
			for _, f := range funcs {
				names = append(names, f.Name)
			}
			return nil, fmt.Errorf("-only: no diagram %q (have %s)", n, strings.Join(names, ", "))
		}
		if !slices.ContainsFunc(out, func(f diagramFunc) bool { return f.Func == funcs[i].Func }) {
			out = append(out, funcs[i])
		}
	}
	return out, nil
}

func cmdRender(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("render")
	outDir := fs.String("o", "diagrams", "output `dir`ectory")
	formats := formatList{"svg"}
	fs.Var(&formats, "f", "comma-separated `formats`: svg, png, html, drawio")
	only := fs.String("only", "", "render only these comma-separated `names` (output or function names)")
	pkg, err := parseArgs(fs, args, stderr)
	if err != nil {
		return err
	}
	p, err := load(ctx, pkg, stderr)
	if err != nil {
		return err
	}
	funcs, err := selectFuncs(p.Funcs, *only)
	if err != nil {
		return err
	}
	if len(funcs) == 0 {
		fmt.Fprintf(stderr, "no diagram functions (func() *diagram.Diagram) in %s\n", pkg)
		return nil
	}
	out, err := filepath.Abs(*outDir)
	if err != nil {
		return err
	}
	src, err := genRunner(p.ImportPath, funcs)
	if err != nil {
		return err
	}
	dir, err := runnerDir(p.ModDir)
	if err != nil {
		return err
	}
	defer removeRunner(dir)
	r, err := execRunner(ctx, p.target, dir, src, out, formats)
	if err != nil {
		return err
	}
	wd, _ := os.Getwd()
	if r.Report == nil {
		fmt.Fprintln(stderr, rebase(r.Output, p.Dir, wd))
		return errFailed
	}
	if r.Output != "" {
		fmt.Fprintln(stderr, rebase(r.Output, p.Dir, wd)) // the diagram functions' own output
	}
	posOf := map[string]string{}
	for _, f := range funcs {
		posOf[f.Func] = f.Pos
	}
	for _, d := range r.Report.Diagrams {
		for _, f := range d.Files {
			fmt.Fprintf(stdout, "wrote %s\n", displayPath(f))
		}
		if len(d.Errors) > 0 {
			fmt.Fprintf(stderr, "%s (%s, %s):\n", d.Name, d.Func, rebase(posOf[d.Func], p.Dir, wd))
			for _, e := range d.Errors {
				e = rebase(strings.TrimRight(e, "\n"), p.Dir, wd)
				fmt.Fprintf(stderr, "\t%s\n", strings.ReplaceAll(e, "\n", "\n\t"))
			}
		}
	}
	if r.Failed() {
		return errFailed
	}
	return nil
}

// posRE matches a relative file:line position, as in compiler output
// ("./payments.go:12:3") and the library's errors ("payments.go:9:").
var posRE = regexp.MustCompile(`(^|[\s(])(?:\./)?([A-Za-z0-9_.\-][A-Za-z0-9_.\-/]*\.go):(\d+)`)

// rebase rewrites positions in text that are relative to dir, and name an
// existing file there, to be relative to wd instead, so they can be
// followed from where the command was run.
func rebase(text, dir, wd string) string {
	rel, err := filepath.Rel(wd, dir)
	if err != nil || rel == "." || wd == "" {
		return text
	}
	return posRE.ReplaceAllStringFunc(text, func(m string) string {
		sub := posRE.FindStringSubmatch(m)
		if _, err := os.Stat(filepath.Join(dir, sub[2])); err != nil {
			return m
		}
		return sub[1] + filepath.Join(rel, sub[2]) + ":" + sub[3]
	})
}

// displayPath shortens path relative to the working directory when it is
// inside it.
func displayPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
