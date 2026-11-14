// Package icons loads SVG and PNG icons for use in diagrams.
//
// Any well-formed SVG loads. SVG and draw.io output embed it unchanged; PNG
// output rasterizes a defined subset of SVG and reports anything outside it
// as an error at render time. CheckPNG runs that check early.
//
// Bundled icon sets live in subpackages (icons/generic, icons/network,
// icons/brands), so a program only carries the sets it imports.
package icons

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/grindlemire/diagram/internal/svgicon"
)

// Kind is an icon's source format.
type Kind uint8

const (
	SVG Kind = iota + 1
	PNG
)

// Icon is an immutable, loaded icon. Icons are safe to share between
// diagrams and goroutines.
type Icon struct {
	name    string
	kind    Kind
	data    []byte
	w, h    float64     // intrinsic size, which fixes the aspect ratio
	viewBox [4]float64  // SVG only
	img     image.Image // PNG only
	mono    bool        // SVG uses currentColor
}

// FromSVG reads an SVG icon. It accepts any well-formed SVG whose root
// element is <svg> and which has a viewBox or a width and height.
func FromSVG(r io.Reader) (*Icon, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return parseSVG("", data)
}

// FromPNG reads a PNG icon.
func FromPNG(r io.Reader) (*Icon, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("icons: decode png: %w", err)
	}
	b := img.Bounds()
	return &Icon{kind: PNG, data: data, img: img, w: float64(b.Dx()), h: float64(b.Dy())}, nil
}

// Open loads an .svg or .png file and names the icon after the file.
func Open(path string) (*Icon, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ic *Icon
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		ic, err = FromSVG(f)
	case ".png":
		ic, err = FromPNG(f)
	default:
		return nil, fmt.Errorf("icons: %s: unsupported extension (want .svg or .png)", path)
	}
	if err != nil {
		return nil, fmt.Errorf("icons: %s: %w", path, err)
	}
	ic.name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return ic, nil
}

// MustSVG parses SVG source that is known to be valid, such as a bundled
// icon, and panics if it isn't.
func MustSVG(name, src string) *Icon {
	ic, err := parseSVG(name, []byte(src))
	if err != nil {
		panic(fmt.Sprintf("icons: bundled icon %q: %v", name, err))
	}
	return ic
}

// Named returns a copy of the icon with a name, used in error messages.
func (i *Icon) Named(name string) *Icon {
	c := *i
	c.name = name
	return &c
}

// Name returns the icon's name, or "" if it has none.
func (i *Icon) Name() string { return i.name }

// Kind returns the icon's source format.
func (i *Icon) Kind() Kind { return i.kind }

// Data returns the icon's original bytes. Callers must not modify them.
func (i *Icon) Data() []byte { return i.data }

// Size returns the icon's intrinsic width and height.
func (i *Icon) Size() (w, h float64) { return i.w, i.h }

// ViewBox returns an SVG icon's viewBox (min-x, min-y, width, height).
func (i *Icon) ViewBox() [4]float64 { return i.viewBox }

// Image returns a PNG icon's decoded image, or nil for SVG icons.
func (i *Icon) Image() image.Image { return i.img }

// Monochrome reports whether an SVG icon paints with currentColor, which
// means a diagram can recolour it.
func (i *Icon) Monochrome() bool { return i.mono }

// CheckPNG reports whether PNG output can draw this icon. PNG icons always
// can; SVG icons can when they stay inside the supported subset.
func (i *Icon) CheckPNG() error {
	if i.kind == PNG {
		return nil
	}
	if _, err := svgicon.Parse(i.data); err != nil {
		if i.name != "" {
			return fmt.Errorf("icon %q: %w", i.name, err)
		}
		return err
	}
	return nil
}

func parseSVG(name string, data []byte) (*Icon, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root *xml.StartElement
	for root == nil {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, errors.New("icons: no <svg> element")
		}
		if err != nil {
			return nil, fmt.Errorf("icons: malformed svg: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local != "svg" {
				return nil, fmt.Errorf("icons: root element is <%s>, want <svg>", se.Name.Local)
			}
			se := se.Copy()
			root = &se
		}
	}
	// Check the rest is well formed.
	for {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("icons: malformed svg: %w", err)
		}
	}

	ic := &Icon{name: name, kind: SVG, data: data}
	var width, height float64
	for _, a := range root.Attr {
		switch a.Name.Local {
		case "viewBox":
			f := strings.FieldsFunc(a.Value, func(r rune) bool { return r == ' ' || r == ',' })
			if len(f) != 4 {
				return nil, fmt.Errorf("icons: bad viewBox %q", a.Value)
			}
			for k := range f {
				v, err := strconv.ParseFloat(f[k], 64)
				if err != nil {
					return nil, fmt.Errorf("icons: bad viewBox %q", a.Value)
				}
				ic.viewBox[k] = v
			}
		case "width":
			width = parseLength(a.Value)
		case "height":
			height = parseLength(a.Value)
		}
	}
	switch {
	case ic.viewBox[2] > 0 && ic.viewBox[3] > 0:
		ic.w, ic.h = ic.viewBox[2], ic.viewBox[3]
	case width > 0 && height > 0:
		ic.w, ic.h = width, height
		ic.viewBox = [4]float64{0, 0, width, height}
	default:
		return nil, errors.New("icons: svg needs a viewBox or a width and height")
	}
	ic.mono = bytes.Contains(data, []byte("currentColor"))
	return ic, nil
}

// parseLength reads a plain or px length; percentages and other units give 0.
func parseLength(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "px")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
