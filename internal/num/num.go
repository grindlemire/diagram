// Package num formats numbers deterministically for output files.
package num

import (
	"math"
	"strconv"
)

// Round rounds v to 2 decimal places.
func Round(v float64) float64 {
	r := math.Round(v*100) / 100
	if r == 0 {
		return 0 // avoid "-0"
	}
	return r
}

// Fmt formats v rounded to 2 decimal places with no trailing zeros.
// Every number written to SVG or draw.io output goes through Fmt, so output
// is byte-identical across runs and machines.
func Fmt(v float64) string {
	return strconv.FormatFloat(Round(v), 'f', -1, 64)
}
