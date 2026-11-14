package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.go": `package pkg

import (
	"github.com/grindlemire/diagram"
	other "example.com/other"
)

type R struct{}

func PaymentsFlow() *diagram.Diagram { return nil }

// Named results are the same signature.
func Named() (d *diagram.Diagram) { return nil }

func unexported() *diagram.Diagram          { return nil }
func WithArg(x int) *diagram.Diagram        { return nil }
func Value() diagram.Diagram                { return diagram.Diagram{} }
func Multi() (*diagram.Diagram, error)      { return nil, nil }
func (R) Method() *diagram.Diagram          { return nil }
func Generic[T any]() *diagram.Diagram      { return nil }
func Other() *other.Diagram                 { return nil }
func NotDiagram() *diagram.Node             { return nil }
var Var = func() *diagram.Diagram { return nil }
`,
		"b.go": `package pkg

import dg "github.com/grindlemire/diagram"

func HTTPServer() *dg.Diagram { return nil }

// diagram is not the package's name in this file.
func Wrong() *diagram.Diagram { return nil }
`,
		"c.go": `package pkg

import . "github.com/grindlemire/diagram"

func Dotted() *Diagram { return nil }
`,
		"d_test.go": `package pkg

import "github.com/grindlemire/diagram"

func InTest() *diagram.Diagram { return nil }
`,
		"e.go": `//go:build ignore

package pkg

import "github.com/grindlemire/diagram"

func Ignored() *diagram.Diagram { return nil }
`,
		"f.go": `package pkg

import _ "github.com/grindlemire/diagram"
import diagram "example.com/fake"

func Fake() *diagram.Diagram { return nil }
`,
		"notes.txt": "func Text() *diagram.Diagram",
	})
	got, err := discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []diagramFunc{
		{Func: "HTTPServer", Name: "http-server", Pos: "b.go:5"},
		{Func: "Named", Name: "named", Pos: "a.go:13"},
		{Func: "PaymentsFlow", Name: "payments-flow", Pos: "a.go:10"},
	}
	if !reflect.DeepEqual(got.Funcs, want) {
		t.Errorf("funcs:\n got %+v\nwant %+v", got.Funcs, want)
	}
	if got.Package != "pkg" {
		t.Errorf("package = %q, want pkg", got.Package)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "c.go:3: ") || !strings.Contains(got.Warnings[0], "dot-imported") {
		t.Errorf("warnings = %q, want one dot-import warning for c.go:3", got.Warnings)
	}
}

func TestDiscoverErrors(t *testing.T) {
	t.Run("collision", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"a.go": `package pkg

import "github.com/grindlemire/diagram"

func HTTPServer() *diagram.Diagram { return nil }
func HttpServer() *diagram.Diagram { return nil }
`})
		_, err := discover(dir)
		if err == nil || !strings.Contains(err.Error(), `both produce "http-server"`) {
			t.Fatalf("err = %v, want a name collision", err)
		}
	})
	t.Run("syntax", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"a.go": "package pkg\n\nfunc {\n"})
		_, err := discover(dir)
		if err == nil || !strings.HasPrefix(err.Error(), "a.go:3:") {
			t.Fatalf("err = %v, want a syntax error at a.go:3", err)
		}
	})
}

func TestKebab(t *testing.T) {
	for in, want := range map[string]string{
		"PaymentsFlow":    "payments-flow",
		"Payments":        "payments",
		"HTTPServer":      "http-server",
		"API":             "api",
		"GetAPIKey":       "get-api-key",
		"OrdersV2":        "orders-v2",
		"V2Orders":        "v2-orders",
		"Flow2":           "flow2",
		"Payments_Flow":   "payments-flow",
		"Payments__Flow_": "payments-flow",
		"X":               "x",
		"ÜberDiagram":     "über-diagram",
	} {
		if got := kebab(in); got != want {
			t.Errorf("kebab(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectFuncs(t *testing.T) {
	funcs := []diagramFunc{{Func: "A", Name: "a"}, {Func: "BigOne", Name: "big-one"}}
	got, err := selectFuncs(funcs, "big-one, A,BigOne")
	if err != nil {
		t.Fatal(err)
	}
	if want := []diagramFunc{funcs[1], funcs[0]}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := selectFuncs(funcs, "nope"); err == nil || !strings.Contains(err.Error(), "have a, big-one") {
		t.Errorf("err = %v, want unknown name listing the names", err)
	}
}

func TestRebase(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "pkg")
	writeFiles(t, pkg, map[string]string{"flow.go": "package pkg\n"})
	in := "./flow.go:3:2: undefined: x\nflow.go:9: node \"B\": cell taken by \"A\" (flow.go:8)\nmissing.go:1: kept\n/abs/flow.go:4: kept"
	want := "pkg/flow.go:3:2: undefined: x\npkg/flow.go:9: node \"B\": cell taken by \"A\" (pkg/flow.go:8)\nmissing.go:1: kept\n/abs/flow.go:4: kept"
	if got := rebase(in, pkg, root); got != want {
		t.Errorf("rebase:\n got %q\nwant %q", got, want)
	}
	if got := rebase(in, pkg, pkg); got != in {
		t.Errorf("rebase to the same directory changed the text: %q", got)
	}
}

func TestParseArgs(t *testing.T) {
	fs := newFlagSet("render")
	o := fs.String("o", "diagrams", "")
	pkg, err := parseArgs(fs, []string{"./pkg", "-o", "out"}, os.Stderr)
	if err != nil || pkg != "./pkg" || *o != "out" {
		t.Errorf("got pkg %q, -o %q, err %v", pkg, *o, err)
	}
	var f formatList
	if err := f.Set("svg, .PNG,svg"); err != nil || f.String() != "svg,png" {
		t.Errorf("formats %q, err %v", f.String(), err)
	}
	if err := f.Set("pdf"); err == nil {
		t.Error("format pdf accepted")
	}
}
