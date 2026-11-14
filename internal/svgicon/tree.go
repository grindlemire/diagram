package svgicon

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	nsSVG   = "http://www.w3.org/2000/svg"
	nsXLink = "http://www.w3.org/1999/xlink"
)

// node is an element of the parsed document.
type node struct {
	name      string
	foreign   bool              // element from a non-SVG namespace: never rendered
	attrs     map[string]string // un-namespaced attributes
	xlinkHref string
	children  []*node
	parent    *node
	text      []byte // character data, kept for <style> only
}

// ref returns the element's href, preferring href over xlink:href as SVG 2
// does.
func (n *node) ref() string {
	if h, ok := n.attrs["href"]; ok {
		return strings.TrimSpace(h)
	}
	return strings.TrimSpace(n.xlinkHref)
}

func buildTree(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root, cur *node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svgicon: malformed svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root != nil && cur == nil {
				return nil, errors.New("svgicon: malformed svg: content after root element")
			}
			n := &node{
				name:    t.Name.Local,
				foreign: t.Name.Space != "" && t.Name.Space != nsSVG,
				attrs:   make(map[string]string, len(t.Attr)),
				parent:  cur,
			}
			for _, a := range t.Attr {
				switch {
				case a.Name.Space == "" || a.Name.Space == nsSVG:
					if a.Name.Local != "xmlns" {
						n.attrs[a.Name.Local] = a.Value
					}
				case a.Name.Local == "href" && (a.Name.Space == nsXLink || a.Name.Space == "xlink"):
					n.xlinkHref = a.Value
				}
			}
			if cur == nil {
				root = n
			} else {
				cur.children = append(cur.children, n)
			}
			cur = n
		case xml.EndElement:
			cur = cur.parent
		case xml.CharData:
			if cur != nil && cur.name == "style" {
				cur.text = append(cur.text, t...)
			}
		}
	}
	if root == nil {
		return nil, errors.New("svgicon: no <svg> element")
	}
	if root.name != "svg" || root.foreign {
		return nil, fmt.Errorf("svgicon: root element is <%s>, want <svg>", root.name)
	}
	return root, nil
}

// supported lists the SVG elements the parser understands. Every other SVG
// element is an error wherever it appears.
var supported = map[string]bool{
	"svg": true, "g": true, "a": true, "defs": true, "symbol": true, "use": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true,
	"linearGradient": true, "radialGradient": true, "stop": true,
	"title": true, "desc": true, "metadata": true, "view": true,
}

// ignoredSubtree reports elements whose content never renders and isn't
// checked: descriptive elements and foreign-namespace elements (editor
// metadata such as sodipodi:namedview).
func ignoredSubtree(n *node) bool {
	if n.foreign {
		return true
	}
	switch n.name {
	case "title", "desc", "metadata":
		return true
	}
	return false
}

// check rejects unsupported elements anywhere in the document and collects
// ids (first one wins, as in browsers).
func check(n *node, ids map[string]*node) error {
	if ignoredSubtree(n) {
		return nil
	}
	if !supported[n.name] {
		// An empty <style> changes nothing.
		if n.name != "style" || len(bytes.TrimSpace(n.text)) > 0 {
			return unsupported(n.name, n)
		}
	}
	if id, ok := n.attrs["id"]; ok {
		if _, dup := ids[id]; !dup {
			ids[id] = n
		}
	}
	for _, c := range n.children {
		if err := check(c, ids); err != nil {
			return err
		}
	}
	return nil
}
