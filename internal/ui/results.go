package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/dawsonxiong/thock/internal/chart"
	"github.com/dawsonxiong/thock/internal/layout"
)

// chartHeight trades vertical space for resolution. Four rows saturates into a
// solid block once speeds cluster; six keeps the variation legible.
const chartHeight = 6

// axisPad aligns rows under the plot with the four-wide label plus " │".
const axisPad = "      "

func (m *Model) resultRows(box int) []string {
	r := m.res
	var rows []string

	rows = append(rows, m.paint(m.tbl.Dim, strings.Repeat("─", box)))
	rows = append(rows, "")

	// The headline pair: figure in the accent, unit muted, so the eye lands on
	// the number without the label competing.
	pb := ""
	if m.isPB {
		pb = "   " + m.paint(m.tbl.Accent, "✦ pb")
	}
	rows = append(rows,
		m.figure(fmt.Sprintf("%.0f", r.WPM), "wpm")+pb,
		m.figure(fmt.Sprintf("%.0f%%", r.Accuracy), "acc"),
		"")

	rows = append(rows, m.chartRows(box)...)
	rows = append(rows, "")

	detail := fmt.Sprintf("raw %.0f · cons %.0f%% · chars %d/%d/%d/%d",
		r.Raw, r.Consistency, r.CorrectChars, r.IncorrectChars, r.ExtraChars, r.MissedChars)
	rows = append(rows, m.paint(m.tbl.Dim, detail))

	var setup string
	switch m.opts.Mode {
	case ModeTime:
		setup = fmt.Sprintf("%ds · english %s", m.opts.Duration, m.opts.List)
	case ModeWords:
		setup = fmt.Sprintf("%d words in %.1fs · english %s",
			m.opts.Words, r.Duration.Seconds(), m.opts.List)
	default:
		setup = "quote · " + string(m.opts.Length)
	}
	rows = append(rows, m.paint(m.tbl.Dim, setup))

	if m.opts.Mode == ModeQuotes && m.quote.Source != "" {
		rows = append(rows, "", m.paint(m.tbl.Text, "— "+m.quote.Attribution()))
	}

	rows = append(rows, "", m.paint(m.tbl.Dim, "enter next · esc repeat · tab options · ctrl+c quit"))
	return rows
}

// figure lays out one headline statistic: the value right-aligned in a fixed
// column so wpm and accuracy line up regardless of their digit counts.
func (m *Model) figure(value, label string) string {
	pad := 6 - layout.Width(value)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + m.paint(m.tbl.Accent, value) +
		"   " + m.paint(m.tbl.Dim, label)
}

// chartRows draws per-second raw speed as columns, with a marker row beneath
// showing which seconds contained mistakes.
func (m *Model) chartRows(box int) []string {
	s := m.res.Samples
	if len(s) < 3 {
		return nil
	}
	const axisW = 5
	plotW := box - axisW - 1
	if plotW < 12 {
		return nil
	}

	errs := make([]int, len(s))
	peak, trough := 0.0, math.Inf(1)
	for i, x := range s {
		errs[i] = x.Errors
		peak = math.Max(peak, x.Raw)
		trough = math.Min(trough, x.Raw)
	}

	// Anchoring at zero turns a steady run into a solid block, so the axis
	// starts just below the slowest second instead. Both bounds are labelled,
	// so the zoom is stated rather than implied.
	// The ceiling is the peak itself so the fastest second reaches the top row;
	// rounding it up would leave the first row permanently blank.
	top := peak
	floor := roundDownTo(trough, 20)
	if top-floor < 20 {
		floor = math.Max(0, top-20)
	}

	raw := make([]float64, len(s))
	for i, x := range s {
		raw[i] = math.Max(0, x.Raw-floor)
	}

	// A short test has only a handful of seconds in it. Rather than drop the
	// chart, give each second an equal block of columns so the bars are wide
	// instead of numerous. Whole-number widths keep every bar the same size,
	// which is what stops it reading as invented detail.
	cols := plotW
	if len(raw) <= plotW {
		per := plotW / len(raw)
		cols = per * len(raw)
		wide := make([]float64, 0, cols)
		for _, v := range raw {
			for i := 0; i < per; i++ {
				wide = append(wide, v)
			}
		}
		raw = wide
	}
	bars := chart.Bars(raw, cols, chartHeight, top-floor)
	out := make([]string, 0, chartHeight+2)
	for i, b := range bars {
		// Label the top and bottom bands only; a label on every row is noise.
		// Unlabelled rows still reserve the width so the axis stays straight.
		lbl := "    "
		switch i {
		case 0:
			lbl = fmt.Sprintf("%4.0f", top)
		case chartHeight - 1:
			lbl = fmt.Sprintf("%4.0f", floor)
		}
		out = append(out, m.paint(m.tbl.Dim, lbl+" │")+m.paint(m.tbl.Accent, b))
	}
	out = append(out, m.paint(m.tbl.Dim, axisPad+strings.Repeat("─", cols)))

	if mark := chart.Marks(errs, cols, '^'); strings.TrimSpace(mark) != "" {
		out = append(out, m.paint(m.tbl.Char[2], axisPad+mark))
	}
	return out
}

func roundDownTo(v, step float64) float64 {
	if step <= 0 || v <= 0 {
		return 0
	}
	return math.Floor(v/step) * step
}
