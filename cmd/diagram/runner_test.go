package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/")

func TestGenRunnerGolden(t *testing.T) {
	src, err := genRunner("example.com/shop/internal/diagrams", []diagramFunc{
		{Func: "HTTPServer", Name: "http-server", Pos: "b.go:5"},
		{Func: "PaymentsFlow", Name: "payments-flow", Pos: "a.go:10"},
	})
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "runner.go.golden")
	if *update {
		if err := os.WriteFile(golden, src, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./cmd/diagram -run TestGenRunnerGolden -update)", err)
	}
	if !bytes.Equal(src, want) {
		t.Errorf("generated runner differs from %s; run with -update to accept:\n%s", golden, src)
	}
}

func TestInjectReload(t *testing.T) {
	got := string(injectReload([]byte("<html><body><p>x</p></body></html>"), 7))
	if !bytes.Contains([]byte(got), []byte("const gen = 7;")) {
		t.Errorf("no generation in %s", got)
	}
	if i, j := bytes.Index([]byte(got), []byte("EventSource")), bytes.Index([]byte(got), []byte("</body>")); i < 0 || j < i {
		t.Errorf("script not before </body>: %s", got)
	}
	if got := string(injectReload([]byte("<p>no body</p>"), 1)); !bytes.Contains([]byte(got), []byte("EventSource")) {
		t.Errorf("script not appended: %s", got)
	}
}

func TestCleanOutput(t *testing.T) {
	for in, want := range map[string]string{
		"# pkg\n./a.go:1:1: bad\nexit status 1\n": "# pkg\n./a.go:1:1: bad",
		"exit status 2\n":                         "",
		"hello\n":                                 "hello",
	} {
		if got := cleanOutput(in); got != want {
			t.Errorf("cleanOutput(%q) = %q, want %q", in, got, want)
		}
	}
}
