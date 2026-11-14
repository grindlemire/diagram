package diagram_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/grindlemire/diagram"
)

// component is templ.Component's method set; templ templates render any
// value that has it.
type component interface {
	Render(ctx context.Context, w io.Writer) error
}

// page stands in for a templ template that places a component in a page.
func page(body component) component {
	return renderFunc(func(ctx context.Context, w io.Writer) error {
		io.WriteString(w, "<article>")
		if err := body.Render(ctx, w); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</article>")
		return err
	})
}

type renderFunc func(context.Context, io.Writer) error

func (f renderFunc) Render(ctx context.Context, w io.Writer) error { return f(ctx, w) }

// Embed's result is a templ component: in a .templ file, write
// @diagram.Embed(d). Pages that serve the script themselves, from
// diagram.ScriptHandler, pass diagram.NoScript.
func ExampleEmbed() {
	d := diagram.New("Checkout")
	web := d.Node("Web")
	api := d.Node("API")
	d.Connect(web, api)

	var b strings.Builder
	if err := page(diagram.Embed(d, diagram.NoScript)).Render(context.Background(), &b); err != nil {
		fmt.Println(err)
		return
	}
	html := b.String()
	fmt.Println(strings.HasPrefix(html, "<article><style>diagram-view[data-dg="))
	fmt.Println(strings.Contains(html, `<diagram-view data-dg="`), strings.Contains(html, ` hash="checkout"><template data-diagram>`))
	fmt.Println(strings.HasSuffix(html, "</template></diagram-view>\n</article>"))
	// Output:
	// true
	// true true
	// true
}
