package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pollInterval is how often watch checks the package's files for changes.
const pollInterval = 300 * time.Millisecond

func cmdWatch(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("watch")
	addr := fs.String("addr", "127.0.0.1:0", "listen `address`; port 0 picks a free port")
	outDir := fs.String("o", "", "also write files to this `dir`ectory after every run")
	formats := formatList{"svg"}
	fs.Var(&formats, "f", "comma-separated `formats` written to -o: svg, png, html, drawio")
	pkg, err := parseArgs(fs, args, stderr)
	if err != nil {
		return err
	}
	t, err := resolve(ctx, pkg)
	if err != nil {
		return err
	}
	w := &watcher{t: t, stdout: stdout, stderr: stderr, subs: map[chan event]bool{}}
	if *outDir != "" {
		if w.outDir, err = filepath.Abs(*outDir); err != nil {
			return err
		}
		w.outFormats = formats
	}
	w.wd, _ = os.Getwd()
	if w.dir, err = runnerDir(t.ModDir); err != nil {
		return err
	}
	defer removeRunner(w.dir)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w.ctx = ctx
	srv := &http.Server{Handler: w.handler(), ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	fmt.Fprintf(stdout, "watching %s\nserving http://%s/\n", displayPath(t.Dir), ln.Addr())

	w.loop(ctx)

	shut, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := srv.Shutdown(shut); err != nil {
		srv.Close()
	}
	if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// A watcher rebuilds the target's diagrams when its files change and serves
// the latest results.
type watcher struct {
	t          *target
	dir        string // runner directory; rendered files go in dir/out
	outDir     string // -o, or ""
	outFormats []string
	wd         string
	stdout     io.Writer
	stderr     io.Writer
	ctx        context.Context // cancelled on exit; ends event streams

	mu   sync.Mutex
	cur  state
	subs map[chan event]bool
}

// state is the outcome of the latest run.
type state struct {
	Gen      int  // increments after every run; 0 until the first finishes
	Building bool // a run is in progress
	Err      string
	Diagrams []view
}

// view is one diagram's latest result.
type view struct {
	Name, Func, Pos, Title string
	Errors                 []string
	SVG, HTML              []byte
}

// An event is sent to open pages: "building" when a run starts, "gen" with
// the new generation when it ends.
type event struct {
	kind string
	gen  int
}

// loop runs once, then again whenever the package's files change, until
// ctx is done.
func (w *watcher) loop(ctx context.Context) {
	prev := w.fingerprint()
	w.rebuild(ctx)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		// Taken before the run, so edits made during it trigger another.
		if fp := w.fingerprint(); fp != prev {
			prev = fp
			w.rebuild(ctx)
		}
	}
}

// fingerprint summarises the name, size and modification time of the
// package's non-test .go files and the module's go.mod.
func (w *watcher) fingerprint() string {
	var b strings.Builder
	entries, _ := os.ReadDir(w.t.Dir)
	paths := []string{filepath.Join(w.t.ModDir, "go.mod")}
	for _, e := range entries {
		if n := e.Name(); !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			paths = append(paths, filepath.Join(w.t.Dir, n))
		}
	}
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", p, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
}

// rebuild discovers the diagram functions, runs them and publishes the
// result.
func (w *watcher) rebuild(ctx context.Context) {
	start := time.Now()
	w.mu.Lock()
	w.cur.Building = true
	w.mu.Unlock()
	w.notify(event{kind: "building"})

	next := w.build(ctx)
	if ctx.Err() != nil {
		return
	}
	failed := 0
	for _, d := range next.Diagrams {
		if len(d.Errors) > 0 {
			failed++
		}
	}
	switch {
	case next.Err != "":
		fmt.Fprintf(w.stderr, "%s build failed:\n%s\n", stamp(), next.Err)
	case failed > 0:
		fmt.Fprintf(w.stdout, "%s %d diagrams, %d with errors (%s)\n", stamp(), len(next.Diagrams), failed, since(start))
		for _, d := range next.Diagrams {
			for _, e := range d.Errors {
				fmt.Fprintf(w.stderr, "%s: %s\n", d.Name, e)
			}
		}
	default:
		fmt.Fprintf(w.stdout, "%s %d diagrams (%s)\n", stamp(), len(next.Diagrams), since(start))
	}

	w.mu.Lock()
	next.Gen = w.cur.Gen + 1
	w.cur = next
	w.mu.Unlock()
	w.notify(event{kind: "gen", gen: next.Gen})
}

func stamp() string                    { return time.Now().Format("15:04:05") }
func since(t time.Time) string         { return time.Since(t).Round(time.Millisecond).String() }
func (w *watcher) rel(s string) string { return rebase(s, w.t.Dir, w.wd) }

// build does one run and returns its state, without a generation.
func (w *watcher) build(ctx context.Context) state {
	disc, err := discover(w.t.Dir)
	if err != nil {
		return state{Err: w.rel(err.Error())}
	}
	for _, warn := range disc.Warnings {
		fmt.Fprintf(w.stderr, "warning: %s\n", w.rel(warn))
	}
	if disc.Package == "main" {
		return state{Err: "package main can't be imported; move the diagram functions to another package"}
	}
	if len(disc.Funcs) == 0 {
		return state{}
	}
	src, err := genRunner(w.t.ImportPath, disc.Funcs)
	if err != nil {
		return state{Err: err.Error()}
	}
	cache := filepath.Join(w.dir, "out")
	os.RemoveAll(cache)
	formats := []string{"svg", "html"}
	for _, f := range w.outFormats {
		if !slices.Contains(formats, f) {
			formats = append(formats, f)
		}
	}
	r, err := execRunner(ctx, w.t, w.dir, src, cache, formats)
	if err != nil {
		return state{Err: err.Error()}
	}
	if r.Output != "" && r.Report != nil {
		fmt.Fprintln(w.stderr, w.rel(r.Output)) // the diagram functions' own output
	}
	if r.Report == nil {
		msg := w.rel(r.Output)
		if msg == "" {
			msg = "the runner failed without output"
		}
		return state{Err: msg}
	}
	posOf := map[string]string{}
	for _, f := range disc.Funcs {
		posOf[f.Func] = f.Pos
	}
	var st state
	for _, d := range r.Report.Diagrams {
		v := view{Name: d.Name, Func: d.Func, Pos: w.rel(posOf[d.Func]), Title: d.Title}
		for _, e := range d.Errors {
			v.Errors = append(v.Errors, w.rel(e))
		}
		if len(v.Errors) == 0 {
			v.SVG, _ = os.ReadFile(filepath.Join(cache, d.Name+".svg"))
			v.HTML, _ = os.ReadFile(filepath.Join(cache, d.Name+".html"))
			if err := w.copyOut(cache, d.Name); err != nil {
				v.Errors = append(v.Errors, err.Error())
			}
		}
		st.Diagrams = append(st.Diagrams, v)
	}
	return st
}

// copyOut copies a diagram's -f formats from the cache to -o.
func (w *watcher) copyOut(cache, name string) error {
	if w.outDir == "" {
		return nil
	}
	if err := os.MkdirAll(w.outDir, 0o755); err != nil {
		return err
	}
	for _, f := range w.outFormats {
		data, err := os.ReadFile(filepath.Join(cache, name+"."+f))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(w.outDir, name+"."+f), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// notify sends e to every open page, dropping it for pages that are behind.
func (w *watcher) notify(e event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (w *watcher) snapshot() state {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cur
}

func (w *watcher) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", w.serveIndex)
	mux.HandleFunc("GET /d/{file}", w.serveDiagram)
	mux.HandleFunc("GET /events", w.serveEvents)
	return mux
}

func (w *watcher) serveIndex(rw http.ResponseWriter, r *http.Request) {
	st := w.snapshot()
	var b bytes.Buffer
	if err := indexTmpl.Execute(&b, map[string]any{
		"S":      st,
		"Pkg":    displayPath(w.t.Dir),
		"Reload": reloadScript(st.Gen),
	}); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	writePage(rw, "text/html; charset=utf-8", b.Bytes())
}

// serveDiagram serves /d/<name> as the interactive page, with the reload
// script added, or an error page, and /d/<name>.svg as SVG.
func (w *watcher) serveDiagram(rw http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	name, isSVG := strings.CutSuffix(file, ".svg")
	st := w.snapshot()
	i := slices.IndexFunc(st.Diagrams, func(v view) bool { return v.Name == name })
	if isSVG {
		if i < 0 || st.Diagrams[i].SVG == nil {
			http.NotFound(rw, r)
			return
		}
		writePage(rw, "image/svg+xml", st.Diagrams[i].SVG)
		return
	}
	var page []byte
	switch {
	case st.Err != "":
		page = w.errorPage(st, "Build failed", "", st.Err)
	case i < 0 && st.Gen == 0:
		page = w.errorPage(st, "Building…", "", "")
	case i < 0:
		rw.WriteHeader(http.StatusNotFound)
		page = w.errorPage(st, "No diagram "+strconv.Quote(name), "", "The package has no diagram function with that name. It may have been renamed or removed.")
	case len(st.Diagrams[i].Errors) > 0:
		v := st.Diagrams[i]
		page = w.errorPage(st, v.Name+" has errors", v.Func+" at "+v.Pos, strings.Join(v.Errors, "\n\n"))
	default:
		page = injectReload(st.Diagrams[i].HTML, st.Gen)
	}
	writePage(rw, "text/html; charset=utf-8", page)
}

func writePage(rw http.ResponseWriter, ctype string, body []byte) {
	rw.Header().Set("Content-Type", ctype)
	rw.Header().Set("Cache-Control", "no-store")
	rw.Write(body)
}

// serveEvents streams events to a page with Server-Sent Events. The current
// generation is sent first, so a page that missed a run while disconnected
// reloads as soon as it reconnects.
func (w *watcher) serveEvents(rw http.ResponseWriter, r *http.Request) {
	fl, ok := rw.(http.Flusher)
	if !ok {
		http.Error(rw, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan event, 8)
	w.mu.Lock()
	w.subs[ch] = true
	cur := w.cur
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.subs, ch)
		w.mu.Unlock()
	}()
	h := rw.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	if cur.Building {
		fmt.Fprint(rw, "event: building\ndata: \n\n")
	}
	fmt.Fprintf(rw, "retry: 1000\nevent: gen\ndata: %d\n\n", cur.Gen)
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-w.ctx.Done():
			return
		case <-ping.C:
			fmt.Fprint(rw, ": ping\n\n")
		case e := <-ch:
			if e.kind == "gen" {
				fmt.Fprintf(rw, "event: gen\ndata: %d\n\n", e.gen)
			} else {
				fmt.Fprintf(rw, "event: %s\ndata: \n\n", e.kind)
			}
		}
		fl.Flush()
	}
}

// reloadScript returns a script that reloads the page when a run newer than
// gen finishes, and shows a badge while a run is in progress or the server
// is unreachable. A reload keeps the URL, including its #fragment.
func reloadScript(gen int) template.HTML {
	return template.HTML(strings.Replace(reloadJS, "GEN", strconv.Itoa(gen), 1))
}

const reloadJS = `<script>
(() => {
  const gen = GEN;
  const badge = document.createElement('div');
  badge.style.cssText = 'position:fixed;left:50%;bottom:12px;transform:translateX(-50%);z-index:2147483647;' +
    'font:12px/1.4 system-ui,sans-serif;padding:4px 10px;border-radius:999px;background:#0f172a;color:#f8fafc;' +
    'box-shadow:0 2px 8px rgba(0,0,0,.25);pointer-events:none;display:none';
  const show = text => { badge.textContent = text; badge.style.display = text ? 'block' : 'none'; };
  addEventListener('DOMContentLoaded', () => document.body.appendChild(badge));
  const es = new EventSource('/events');
  es.addEventListener('building', () => show('rebuilding…'));
  es.addEventListener('gen', e => { if (+e.data !== gen) location.reload(); else show(''); });
  es.onerror = () => show('diagram watch is not running');
})();
</script>
`

// injectReload adds the reload script to an HTML page, before its last
// </body>.
func injectReload(page []byte, gen int) []byte {
	script := []byte(reloadScript(gen))
	i := bytes.LastIndex(page, []byte("</body>"))
	if i < 0 {
		return append(append([]byte{}, page...), script...)
	}
	out := make([]byte, 0, len(page)+len(script))
	out = append(out, page[:i]...)
	out = append(out, script...)
	return append(out, page[i:]...)
}

func (w *watcher) errorPage(st state, title, sub, detail string) []byte {
	var b bytes.Buffer
	errorTmpl.Execute(&b, map[string]any{
		"Title": title, "Sub": sub, "Detail": detail,
		"Pkg": displayPath(w.t.Dir), "Reload": reloadScript(st.Gen),
	})
	return b.Bytes()
}

const pageCSS = `
:root{color-scheme:light dark;--bg:#f8fafc;--card:#fff;--fg:#0f172a;--muted:#64748b;--line:#e2e8f0;--accent:#4f46e5;--err:#b91c1c;--errbg:#fef2f2}
@media (prefers-color-scheme:dark){:root{--bg:#0b1120;--card:#111827;--fg:#e5e7eb;--muted:#94a3b8;--line:#1f2937;--accent:#818cf8;--err:#fca5a5;--errbg:#2a1215}}
*{box-sizing:border-box}
body{margin:0;font:14px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;background:var(--bg);color:var(--fg)}
header{padding:20px 24px 4px}
h1{margin:0;font-size:20px}
header p{margin:2px 0 0;color:var(--muted)}
main{padding:16px 24px 48px}
pre{margin:0;white-space:pre-wrap;word-break:break-word;font:12.5px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace}
.err{background:var(--errbg);color:var(--err);border:1px solid var(--err);border-radius:10px;padding:14px 16px}
a{color:var(--accent)}
`

var indexTmpl = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="icon" href="data:,">
<title>diagrams · {{.Pkg}}</title>
<style>` + pageCSS + `
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:16px}
.card{display:flex;flex-direction:column;background:var(--card);border:1px solid var(--line);border-radius:10px;overflow:hidden;color:inherit;text-decoration:none}
a.card:hover{border-color:var(--accent)}
.thumb{height:200px;display:flex;align-items:center;justify-content:center;padding:12px;background:#fff;border-bottom:1px solid var(--line)}
.thumb img{max-width:100%;max-height:100%}
.meta{padding:10px 14px}
.meta b{display:block}
.meta span{color:var(--muted);font-size:12px}
.card .err{border:0;border-radius:0;border-bottom:1px solid var(--line);height:200px;overflow:auto}
.empty{color:var(--muted)}
</style>
<header>
  <h1>Diagrams</h1>
  <p>{{.Pkg}} · {{if .S.Building}}rebuilding…{{else if .S.Gen}}live: pages reload when you save{{else}}building…{{end}}</p>
</header>
<main>
{{if .S.Err}}<div class="err"><b>Build failed</b><pre>{{.S.Err}}</pre></div>
{{else if not .S.Diagrams}}<p class="empty">{{if .S.Gen}}No diagram functions found. Declare an exported func() *diagram.Diagram in this package.{{else}}Building…{{end}}</p>
{{else}}<div class="grid">
{{range .S.Diagrams}}{{if .Errors}}
  <a class="card" href="/d/{{.Name}}"><div class="err"><pre>{{range .Errors}}{{.}}
{{end}}</pre></div><div class="meta"><b>{{.Name}}</b><span>{{.Func}} · {{.Pos}} · has errors</span></div></a>
{{else}}
  <a class="card" href="/d/{{.Name}}"><div class="thumb"><img src="/d/{{.Name}}.svg?gen={{$.S.Gen}}" alt="{{.Title}}" loading="lazy"></div><div class="meta"><b>{{if .Title}}{{.Title}}{{else}}{{.Name}}{{end}}</b><span>{{.Name}} · {{.Func}} · {{.Pos}}</span></div></a>
{{end}}{{end}}
</div>{{end}}
</main>
{{.Reload}}
</html>
`))

var errorTmpl = template.Must(template.New("error").Parse(`<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="icon" href="data:,">
<title>{{.Title}}</title>
<style>` + pageCSS + `</style>
<body>
<header>
  <h1>{{.Title}}</h1>
  <p>{{if .Sub}}{{.Sub}} · {{end}}{{.Pkg}} · <a href="/">all diagrams</a> · this page reloads when the problem is fixed</p>
</header>
<main>{{if .Detail}}<div class="err"><pre>{{.Detail}}</pre></div>{{end}}</main>
{{.Reload}}
</body>
</html>
`))
