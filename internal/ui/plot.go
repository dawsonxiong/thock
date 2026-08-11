package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/dawsonxiong/thock/internal/chart"
)

const (
	// axisW is the width of the y-axis label column, and axisPad the same width
	// in blanks for rows that sit under the plot rather than beside it.
	axisW   = 5
	axisPad = "      "
)

// plotWidth is how many columns a plot gets inside a box, or zero when the box
// is too narrow for one to say anything.
func plotWidth(box int) int {
	w := box - axisW - 1
	if w < 12 {
		return 0
	}
	return w
}

// plot paints a bar chart of vals, which must already be measured from floor.
// Only the top and bottom bands are labelled — a figure on every row is noise —
// but every row reserves the label width so the axis stays straight.
func (m *Model) plot(vals []float64, cols, height int, floor, top float64) []string {
	out := make([]string, 0, height+1)
	for i, b := range chart.Bars(vals, cols, height, top-floor) {
		lbl := "    "
		switch i {
		case 0:
			lbl = fmt.Sprintf("%4.0f", top)
		case height - 1:
			lbl = fmt.Sprintf("%4.0f", floor)
		}
		out = append(out, m.paint(m.tbl.Dim, lbl+" │")+m.paint(m.tbl.Accent, b))
	}
	return append(out, m.paint(m.tbl.Dim, axisPad+strings.Repeat("─", cols)))
}

// offset rebases values onto a floor, which is what lets a plot start just below
// the smallest value instead of at zero.
func offset(vals []float64, floor float64) []float64 {
	out := make([]float64, len(vals))
	for i, v := range vals {
		out[i] = math.Max(0, v-floor)
	}
	return out
}
