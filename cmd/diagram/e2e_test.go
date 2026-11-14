package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const flowSrc = `package pkg

import dg "github.com/grindlemire/diagram"

// PaymentsFlow is a diagram.
func PaymentsFlow() *dg.Diagram {
	d := dg.New("Payments flow")
	web := d.Node("Web")
	api := d.Node("API")
	d.Connect(web, api, dg.Label("HTTPS"))
	return d
}

func Storage() *dg.Diagram {
	d := dg.New("Storage")
	d.Node("Postgres", dg.Cylinder)
	return d
}
`

const brokenSrc = `package pkg

import "github.com/grindlemire/diagram"

func Broken() *diagram.Diagram {
	d := diagram.New("Broken")
	g := d.Grid("Workers", 2)
	g.Node("A", 0, 0)
	g.Node("B", 0, 0)
	return d
}
`

// tempModule creates a module that uses this repository's diagram package
// through a replace directive, with flowSrc in package pkg, and returns its
// root. It skips the test under -short, or without a go command.
//
// DIAGRAM_TEST_LIBRARY replaces the library with another checkout of it,
// for example a clean `git archive HEAD` while the tree doesn't build.
func tempModule(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("runs go; skipped with -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	repo, err := filepath.Abs("../..")
	if dir := os.Getenv("DIAGRAM_TEST_LIBRARY"); dir != "" {
		repo, err = filepath.Abs(dir)
	}
	if err != nil {
		t.Fatal(err)
	}
	gomod, err := os.ReadFile(filepath.Join(repo, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	gosum, err := os.ReadFile(filepath.Join(repo, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the repository's go directive and requirements, so the module
	// builds offline from the same module cache and go.sum.
	_, rest, _ := strings.Cut(string(gomod), "\n")
	mod := "module example.com/tmpmod\n" + rest +
		"\nrequire github.com/grindlemire/diagram v0.0.0\n\nreplace github.com/grindlemire/diagram => " + repo + "\n"
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":      mod,
		"go.sum":      string(gosum),
		"pkg/flow.go": flowSrc,
	})
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=readonly")
	return root
}

func TestRenderEndToEnd(t *testing.T) {
	root := tempModule(t)
	t.Chdir(root)

	var out, errOut bytes.Buffer
	code := runCLI(context.Background(), []string{"render", "./pkg", "-o", "out", "-f", "svg,png,html,drawio"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("render exited %d\nstdout:\n%s\nstderr:\n%s", code, &out, &errOut)
	}
	for _, name := range []string{"payments-flow", "storage"} {
		for _, ext := range []string{"svg", "png", "html", "drawio"} {
			p := filepath.Join("out", name+"."+ext)
			fi, err := os.Stat(p)
			if err != nil || fi.Size() == 0 {
				t.Errorf("%s: %v", p, err)
			}
			if !strings.Contains(out.String(), "wrote "+p+"\n") {
				t.Errorf("stdout doesn't report %s:\n%s", p, &out)
			}
		}
	}
	assertNoRunner(t, root)

	t.Run("list", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if code := runCLI(context.Background(), []string{"list", "./pkg"}, &out, &errOut); code != 0 {
			t.Fatalf("list exited %d: %s", code, &errOut)
		}
		want := "payments-flow  PaymentsFlow  pkg/flow.go:6\nstorage        Storage       pkg/flow.go:14\n"
		if out.String() != want {
			t.Errorf("list:\n%s\nwant:\n%s", &out, want)
		}
	})

	t.Run("only", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if code := runCLI(context.Background(), []string{"render", "-o", "only", "-only", "storage", "./pkg"}, &out, &errOut); code != 0 {
			t.Fatalf("render exited %d: %s", code, &errOut)
		}
		if out.String() != "wrote only/storage.svg\n" {
			t.Errorf("stdout = %q", &out)
		}
	})

	t.Run("invalid diagram", func(t *testing.T) {
		writeFiles(t, root, map[string]string{"pkg/broken.go": brokenSrc})
		defer os.Remove(filepath.Join(root, "pkg/broken.go"))
		os.RemoveAll("out")
		var out, errOut bytes.Buffer
		code := runCLI(context.Background(), []string{"render", "./pkg", "-o", "out"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("render exited %d, want 1\nstdout:\n%s\nstderr:\n%s", code, &out, &errOut)
		}
		// The other diagrams are still rendered.
		for _, p := range []string{"out/payments-flow.svg", "out/storage.svg"} {
			if _, err := os.Stat(p); err != nil {
				t.Error(err)
			}
		}
		if _, err := os.Stat("out/broken.svg"); err == nil {
			t.Error("out/broken.svg written for an invalid diagram")
		}
		for _, want := range []string{"broken (Broken, pkg/broken.go:5):", "pkg/broken.go:9: ", "already occupied", "(pkg/broken.go:8)"} {
			if !strings.Contains(errOut.String(), want) {
				t.Errorf("stderr lacks %q:\n%s", want, &errOut)
			}
		}
		assertNoRunner(t, root)
	})

	t.Run("build error", func(t *testing.T) {
		writeFiles(t, root, map[string]string{"pkg/bad.go": "package pkg\n\nvar _ = undefinedThing\n"})
		defer os.Remove(filepath.Join(root, "pkg/bad.go"))
		var out, errOut bytes.Buffer
		code := runCLI(context.Background(), []string{"render", "./pkg", "-o", "out"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("render exited %d, want 1\nstderr:\n%s", code, &errOut)
		}
		if !strings.Contains(errOut.String(), "pkg/bad.go:3:9: undefined: undefinedThing") {
			t.Errorf("stderr lacks the compiler error:\n%s", &errOut)
		}
		assertNoRunner(t, root)
	})

	t.Run("package main", func(t *testing.T) {
		writeFiles(t, root, map[string]string{"cmd/x/main.go": "package main\n\nfunc main() {}\n"})
		var out, errOut bytes.Buffer
		if code := runCLI(context.Background(), []string{"render", "./cmd/x"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "package main") {
			t.Errorf("exit %d, stderr %s", code, &errOut)
		}
	})
}

func assertNoRunner(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".diagram")); err == nil {
		t.Errorf("%s/.diagram left behind", root)
	}
}

// syncBuffer is a bytes.Buffer safe for one writer and concurrent readers.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestWatchLiveReload(t *testing.T) {
	root := tempModule(t)
	t.Chdir(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr syncBuffer
	exited := make(chan int, 1)
	go func() { exited <- runCLI(ctx, []string{"watch", "./pkg", "-o", "live"}, &stdout, &stderr) }()

	urlRE := regexp.MustCompile(`serving (http://127\.0\.0\.1:\d+)/`)
	var base string
	waitFor(t, "the server URL", func() bool {
		m := urlRE.FindStringSubmatch(stdout.String())
		if m != nil {
			base = m[1]
		}
		return m != nil
	}, &stdout, &stderr)

	events := openEvents(t, ctx, base)
	gen := waitGen(t, events, 1)

	if page := get(t, base+"/"); !strings.Contains(page, `href="/d/payments-flow"`) || !strings.Contains(page, `/d/storage.svg?gen=`) {
		t.Errorf("index lacks the diagrams:\n%s", page)
	}
	page := get(t, base+"/d/payments-flow")
	if !strings.Contains(page, "<svg") || !strings.Contains(page, "new EventSource('/events')") {
		t.Errorf("diagram page lacks the SVG or the reload script")
	}
	if svg := get(t, base+"/d/storage.svg"); !strings.HasPrefix(strings.TrimSpace(svg), "<") || !strings.Contains(svg, "<svg") {
		t.Errorf("/d/storage.svg is not SVG")
	}
	if _, err := os.Stat(filepath.Join(root, "live", "storage.svg")); err != nil {
		t.Errorf("-o: %v", err)
	}

	// A compile error shows the error page, with the compiler's file:line.
	writeFiles(t, root, map[string]string{"pkg/flow.go": strings.Replace(flowSrc, `d.Node("API")`, `d.Node(apiLabel)`, 1)})
	gen = until(t, events, gen, base+"/d/payments-flow", "Build failed", "pkg/flow.go:9:", "undefined: apiLabel")

	// A validation error shows that diagram's errors; the others still render.
	writeFiles(t, root, map[string]string{"pkg/flow.go": flowSrc, "pkg/broken.go": brokenSrc})
	gen = until(t, events, gen, base+"/d/broken", "broken has errors", "pkg/broken.go:9:", "already occupied")
	if page := get(t, base+"/d/storage"); !strings.Contains(page, "<svg") {
		t.Errorf("storage no longer renders")
	}

	// Fixed: the page renders again.
	os.Remove(filepath.Join(root, "pkg", "broken.go"))
	writeFiles(t, root, map[string]string{"pkg/flow.go": strings.Replace(flowSrc, `"API"`, `"Gateway"`, 1)})
	until(t, events, gen, base+"/d/payments-flow", "<svg", "Gateway")
	if page := get(t, base+"/d/broken"); !strings.Contains(page, "No diagram") {
		t.Errorf("removed diagram still served")
	}

	cancel()
	select {
	case code := <-exited:
		if code != 0 {
			t.Errorf("watch exited %d: %s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch did not exit after cancel")
	}
	assertNoRunner(t, root)
}

// openEvents connects to the event stream and returns the generations it
// announces.
func openEvents(t *testing.T, ctx context.Context, base string) <-chan int {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	ch := make(chan int, 16)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		kind := ""
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				kind = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: ") && kind == "gen":
				if n, err := strconv.Atoi(strings.TrimPrefix(line, "data: ")); err == nil {
					ch <- n
				}
			}
		}
		close(ch)
	}()
	return ch
}

// until waits for reload events after gen until the page at url contains
// every string in want, and returns the last generation seen. A change can
// take more than one run when files are written one at a time.
func until(t *testing.T, events <-chan int, gen int, url string, want ...string) int {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		gen = waitGen(t, events, gen+1)
		page := get(t, url)
		ok := true
		for _, w := range want {
			ok = ok && strings.Contains(page, w)
		}
		if ok {
			return gen
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never contained %q; last:\n%s", url, want, page)
		}
	}
}

// waitGen waits for a reload event announcing generation want or later.
func waitGen(t *testing.T, events <-chan int, want int) int {
	t.Helper()
	timeout := time.After(90 * time.Second)
	for {
		select {
		case n, ok := <-events:
			if !ok {
				t.Fatal("event stream closed")
			}
			if n >= want {
				return n
			}
		case <-timeout:
			t.Fatalf("no reload event for generation %d", want)
		}
	}
}

func waitFor(t *testing.T, what string, ok func() bool, logs ...interface{ String() string }) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			var b strings.Builder
			for _, l := range logs {
				b.WriteString(l.String())
			}
			t.Fatalf("timed out waiting for %s\n%s", what, b.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
