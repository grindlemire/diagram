// Package examples holds example diagrams. Showcase examples demonstrate
// the library; stress examples deliberately push on its known weak spots.
// Run the gallery to render them all:
//
//	go run ./examples/gallery
package examples

import "github.com/grindlemire/diagram"

// Kind groups examples in the gallery.
type Kind string

const (
	Showcase Kind = "showcase"
	Stress   Kind = "stress"
)

// Example is one diagram in the gallery.
type Example struct {
	Name  string // file name stem
	Title string
	Kind  Kind
	// Notes says what the example demonstrates or, for stress examples,
	// what to look for.
	Notes string
	Build func() *diagram.Diagram
}

// All lists every example in gallery order.
func All() []Example {
	return append(append([]Example{}, showcase...), stress...)
}
