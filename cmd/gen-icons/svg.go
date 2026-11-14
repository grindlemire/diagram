package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// minifyOpts says how to rewrite an icon beyond the common minification.
type minifyOpts struct {
	dropAttrs    []string // attributes removed from every element
	dropElems    []string // elements removed with their content
	rootAttrs    []xml.Attr
	requireStyle string // a root attribute that must already be present, as name="value"
}

var lucideOpts = minifyOpts{
	dropAttrs:    []string{"class"},
	requireStyle: `stroke="currentColor"`,
}

// Simple Icons paths carry no fill, so they paint black. Filling the root
// with currentColor lets diagrams recolour them. The title and role only
// matter for standalone use and would show as a tooltip inside a diagram.
var simpleIconsOpts = minifyOpts{
	dropAttrs: []string{"role"},
	dropElems: []string{"title"},
	rootAttrs: []xml.Attr{{Name: xml.Name{Local: "fill"}, Value: "currentColor"}},
}

// minify re-serializes an SVG without comments, processing instructions,
// doctype or whitespace-only text, keeping every element and geometry
// attribute byte for byte.
func minify(src []byte, o minifyOpts) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(src))
	var out strings.Builder
	var (
		open    bool // a start tag is written but not yet closed with '>'
		depth   int
		skip    int // depth inside a dropped element, 0 when not skipping
		sawRoot bool
	)
	closeOpen := func() {
		if open {
			out.WriteByte('>')
			open = false
		}
	}
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 {
				skip++
				continue
			}
			if slices.Contains(o.dropElems, t.Name.Local) && t.Name.Space == "" {
				skip = 1
				continue
			}
			closeOpen()
			root := depth == 0
			if root {
				if sawRoot {
					return "", errors.New("more than one root element")
				}
				if t.Name.Local != "svg" || t.Name.Space != "" {
					return "", fmt.Errorf("root element is <%s>, want <svg>", qname(t.Name))
				}
				sawRoot = true
			}
			out.WriteByte('<')
			out.WriteString(qname(t.Name))
			attrs := t.Attr
			if root {
				for _, a := range o.rootAttrs {
					for _, b := range attrs {
						if b.Name == a.Name {
							return "", fmt.Errorf("root already has %s=%q", qname(a.Name), b.Value)
						}
					}
				}
				attrs = append(attrs[:len(attrs):len(attrs)], o.rootAttrs...)
				if o.requireStyle != "" && !hasAttr(attrs, o.requireStyle) {
					return "", fmt.Errorf("root lacks %s", o.requireStyle)
				}
			}
			for _, a := range attrs {
				if a.Name.Space == "" && slices.Contains(o.dropAttrs, a.Name.Local) {
					continue
				}
				out.WriteByte(' ')
				out.WriteString(qname(a.Name))
				out.WriteString(`="`)
				out.WriteString(escapeAttr(a.Value))
				out.WriteByte('"')
			}
			open = true
			depth++
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			depth--
			if open {
				out.WriteString("/>")
				open = false
			} else {
				out.WriteString("</" + qname(t.Name) + ">")
			}
		case xml.CharData:
			if skip > 0 || len(bytes.TrimSpace(t)) == 0 {
				continue
			}
			if depth == 0 {
				return "", errors.New("text outside the root element")
			}
			closeOpen()
			var b bytes.Buffer
			if err := xml.EscapeText(&b, t); err != nil {
				return "", err
			}
			out.Write(b.Bytes())
		case xml.Comment, xml.ProcInst, xml.Directive:
			// dropped
		}
	}
	if !sawRoot || depth != 0 {
		return "", errors.New("no complete <svg> element")
	}
	return out.String(), nil
}

func qname(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

func hasAttr(attrs []xml.Attr, nameValue string) bool {
	for _, a := range attrs {
		if qname(a.Name)+`="`+a.Value+`"` == nameValue {
			return true
		}
	}
	return false
}

var attrEscaper = strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `"`, "&quot;", "\t", "&#x9;", "\n", "&#xA;", "\r", "&#xD;")

func escapeAttr(s string) string { return attrEscaper.Replace(s) }
