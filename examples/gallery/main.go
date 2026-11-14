// Command gallery renders every example as SVG, PNG, interactive HTML and
// draw.io and writes an index.html for reviewing them side by side.
//
//	go run ./examples/gallery -out /tmp/diagram-gallery
//	open /tmp/diagram-gallery/index.html
//
// The draw.io tab uses the official viewer script from viewer.diagrams.net,
// so it needs a network connection.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/grindlemire/diagram/examples"
)

type result struct {
	examples.Example
	Err                            string
	SVGTime, PNGTime, HTime, DTime time.Duration
	SVGSize, PNGSize, HSize, DSize int
	DrawioJSON                     string
}

func main() {
	out := flag.String("out", "gallery-out", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}

	var results []result
	for _, ex := range examples.All() {
		r := result{Example: ex}
		d := ex.Build()
		if err := d.Validate(); err != nil {
			r.Err = err.Error()
			results = append(results, r)
			fmt.Printf("%-16s errors (expected for the errors example)\n", ex.Name)
			continue
		}
		var err error
		var svg, png, page, dio []byte
		if svg, r.SVGTime, err = timed(func(b *bytes.Buffer) error { return d.RenderSVG(b) }); err == nil {
			if png, r.PNGTime, err = timed(func(b *bytes.Buffer) error { return d.RenderPNG(b) }); err == nil {
				if page, r.HTime, err = timed(func(b *bytes.Buffer) error { return d.RenderHTML(b) }); err == nil {
					dio, r.DTime, err = timed(func(b *bytes.Buffer) error { return d.RenderDrawio(b) })
				}
			}
		}
		if err != nil {
			r.Err = err.Error()
			results = append(results, r)
			fmt.Printf("%-16s render error: %v\n", ex.Name, err)
			continue
		}
		r.SVGSize, r.PNGSize, r.HSize, r.DSize = len(svg), len(png), len(page), len(dio)
		for ext, data := range map[string][]byte{".svg": svg, ".png": png, ".html": page, ".drawio": dio} {
			if err := os.WriteFile(filepath.Join(*out, ex.Name+ext), data, 0o644); err != nil {
				log.Fatal(err)
			}
		}
		cfg, _ := json.Marshal(map[string]any{"xml": string(dio), "nav": true, "resize": true, "toolbar": "zoom", "lightbox": false})
		r.DrawioJSON = string(cfg)
		results = append(results, r)
		fmt.Printf("%-16s svg %6s %5.1fKB  png %6s %6.1fKB  html %6s %5.1fKB  drawio %6s %5.1fKB\n", ex.Name,
			ms(r.SVGTime), kb(r.SVGSize), ms(r.PNGTime), kb(r.PNGSize), ms(r.HTime), kb(r.HSize), ms(r.DTime), kb(r.DSize))
	}

	var page bytes.Buffer
	if err := tmpl.Execute(&page, results); err != nil {
		log.Fatal(err)
	}
	index := filepath.Join(*out, "index.html")
	if err := os.WriteFile(index, page.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	abs, _ := filepath.Abs(index)
	fmt.Println("\nwrote", abs)
}

func timed(f func(*bytes.Buffer) error) ([]byte, time.Duration, error) {
	var b bytes.Buffer
	start := time.Now()
	err := f(&b)
	return b.Bytes(), time.Since(start), err
}

func ms(d time.Duration) string { return fmt.Sprintf("%.0fms", float64(d.Microseconds())/1000) }
func kb(n int) float64          { return float64(n) / 1024 }

var tmpl = template.Must(template.New("page").Funcs(template.FuncMap{
	"ms": ms,
	"kb": func(n int) string { return fmt.Sprintf("%.1f KB", kb(n)) },
	"attr": func(s string) template.HTMLAttr {
		return template.HTMLAttr(`data-mxgraph="` + template.HTMLEscapeString(s) + `"`)
	},
}).Parse(`<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>diagram gallery</title>
<style>
:root{--bg:#f1f5f9;--card:#fff;--fg:#0f172a;--muted:#64748b;--line:#e2e8f0;--accent:#4f46e5;--warn:#b45309;--warnbg:#fffbeb}
*{box-sizing:border-box}
body{margin:0;font:14px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;background:var(--bg);color:var(--fg)}
header{padding:24px 24px 8px;max-width:1400px;margin:auto}
h1{margin:0 0 4px;font-size:22px}
header p{margin:0;color:var(--muted)}
nav{display:flex;flex-wrap:wrap;gap:6px;margin-top:12px}
nav a{font-size:12px;padding:3px 9px;border-radius:999px;background:var(--card);border:1px solid var(--line);color:var(--fg);text-decoration:none}
nav a.stress{border-color:#fcd34d;background:var(--warnbg)}
main{max-width:1400px;margin:auto;padding:8px 24px 48px}
h2{margin:32px 0 12px;font-size:16px;text-transform:uppercase;letter-spacing:.06em;color:var(--muted)}
.card{background:var(--card);border:1px solid var(--line);border-radius:10px;margin:0 0 20px;overflow:hidden}
.head{padding:14px 16px;border-bottom:1px solid var(--line)}
.head h3{margin:0;font-size:16px;display:flex;gap:8px;align-items:center}
.badge{font-size:11px;font-weight:600;padding:1px 8px;border-radius:999px;background:#eef2ff;color:var(--accent)}
.badge.stress{background:var(--warnbg);color:var(--warn)}
.notes{margin:6px 0 0;color:#334155;max-width:110ch}
.meta{margin-top:8px;font-size:12px;color:var(--muted);display:flex;flex-wrap:wrap;gap:4px 16px}
.meta a{color:var(--accent)}
.tabs{display:flex;gap:4px;padding:8px 16px 0}
.tabs button{font:inherit;font-size:12px;padding:4px 12px;border:1px solid var(--line);border-bottom:none;border-radius:6px 6px 0 0;background:var(--bg);cursor:pointer;color:var(--muted)}
.tabs button[aria-selected=true]{background:var(--card);color:var(--fg);font-weight:600}
.panes{border-top:1px solid var(--line);padding:16px;overflow:auto;background:
  repeating-conic-gradient(#f8fafc 0 25%,#fff 0 50%) 0 0/16px 16px}
.pane{display:none}.pane.on{display:block}
.pane img{display:block;max-width:100%;height:auto}
.pane iframe{display:block;width:100%;height:560px;border:1px solid var(--line);border-radius:8px;background:var(--card)}
.compare{display:grid;grid-template-columns:1fr 1fr;gap:12px}
.compare figure{margin:0}.compare figcaption{font-size:11px;color:var(--muted);margin-bottom:4px}
.err{margin:0;padding:16px;white-space:pre-wrap;font:12px/1.6 ui-monospace,Menlo,monospace;background:#fef2f2;color:#991b1b}
@media (max-width:800px){.compare{grid-template-columns:1fr}}
</style>
<header>
  <h1>diagram gallery</h1>
  <p>Every example rendered as SVG, PNG, interactive HTML and draw.io. Stress examples point at known weak spots; their notes say what to look for.</p>
  <nav>{{range .}}<a href="#{{.Name}}" class="{{.Kind}}">{{.Title}}</a>{{end}}</nav>
</header>
<main>
{{$kind := ""}}
{{range .}}
{{if ne (print .Kind) $kind}}{{$kind = print .Kind}}<h2>{{if eq $kind "stress"}}Stress tests and known weaknesses{{else}}Showcase{{end}}</h2>{{end}}
<section class="card" id="{{.Name}}">
  <div class="head">
    <h3>{{.Title}} <span class="badge {{.Kind}}">{{.Kind}}</span></h3>
    <p class="notes">{{.Notes}}</p>
    {{if not .Err}}<div class="meta">
      <span>SVG {{ms .SVGTime}} · {{kb .SVGSize}} · <a href="{{.Name}}.svg">open</a></span>
      <span>PNG {{ms .PNGTime}} · {{kb .PNGSize}} · <a href="{{.Name}}.png">open</a></span>
      <span>HTML {{ms .HTime}} · {{kb .HSize}} · <a href="{{.Name}}.html">open</a></span>
      <span>draw.io {{ms .DTime}} · {{kb .DSize}} · <a href="{{.Name}}.drawio" download>download</a> (open in draw.io to edit)</span>
    </div>{{end}}
  </div>
  {{if .Err}}<pre class="err">{{.Err}}</pre>{{else}}
  <div class="tabs" role="tablist">
    <button role="tab" aria-selected="true" data-pane="svg">SVG</button>
    <button role="tab" aria-selected="false" data-pane="png">PNG</button>
    <button role="tab" aria-selected="false" data-pane="html">HTML</button>
    <button role="tab" aria-selected="false" data-pane="drawio">draw.io</button>
    <button role="tab" aria-selected="false" data-pane="compare">SVG vs PNG</button>
  </div>
  <div class="panes">
    <div class="pane on" data-pane="svg"><img src="{{.Name}}.svg" alt="{{.Title}} as SVG"></div>
    <div class="pane" data-pane="png"><img src="{{.Name}}.png" alt="{{.Title}} as PNG" onload="this.width=this.naturalWidth/2"></div>
    <div class="pane" data-pane="html"><iframe data-src="{{.Name}}.html" title="{{.Title}} as interactive HTML" loading="lazy"></iframe></div>
    <div class="pane" data-pane="drawio"><div class="mxgraph-lazy" {{attr .DrawioJSON}}></div></div>
    <div class="pane" data-pane="compare"><div class="compare">
      <figure><figcaption>SVG</figcaption><img src="{{.Name}}.svg" alt=""></figure>
      <figure><figcaption>PNG</figcaption><img src="{{.Name}}.png" alt=""></figure>
    </div></div>
  </div>
  {{end}}
</section>
{{end}}
</main>
<script>
let viewer;
function loadViewer(){
  if(viewer) return viewer;
  viewer = new Promise(res=>{
    window.onDrawioViewerLoad = ()=>res();
    const s=document.createElement('script');
    s.src='https://viewer.diagrams.net/js/viewer-static.min.js';
    s.onload=()=>res();
    document.head.appendChild(s);
  });
  return viewer;
}
document.querySelectorAll('.card').forEach(card=>{
  card.querySelectorAll('.tabs button').forEach(btn=>btn.addEventListener('click',async()=>{
    card.querySelectorAll('.tabs button').forEach(b=>b.setAttribute('aria-selected',b===btn));
    card.querySelectorAll('.pane').forEach(p=>p.classList.toggle('on',p.dataset.pane===btn.dataset.pane));
    if(btn.dataset.pane==='html'){
      const f=card.querySelector('iframe[data-src]');
      if(f&&!f.src) f.src=f.dataset.src;
    }
    if(btn.dataset.pane==='drawio'){
      const el=card.querySelector('.mxgraph-lazy');
      if(el){
        await loadViewer();
        el.className='mxgraph';
        el.style.background='#fff';
        window.GraphViewer && GraphViewer.processElements();
      }
    }
  }));
});
</script>
</html>
`))
