package ui

import (
	"fmt"
	"strings"

	"github.com/dawsonxiong/thock/internal/chart"
	"github.com/dawsonxiong/thock/internal/layout"
)

// chartHeight trades vertical space for resolution. Four rows saturates into a
// solid block once speeds cluster; six keeps the variation legible.
const chartHeight = 6

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

	rows = append(rows, "", m.paint(m.tbl.Dim,
		"enter next · esc repeat · tab options · ctrl+s stats · ctrl+c quit"))
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
	cols := plotWidth(box)
	if cols == 0 {
		return nil
	}

	errs := make([]int, len(s))
	raw := make([]float64, len(s))
	for i, x := range s {
		errs[i] = x.Errors
		raw[i] = x.Raw
	}

	// A short test has only a handful of seconds in it, so each second is given
	// an equal block of columns rather than the chart being dropped.
	floor, top := chart.Frame(raw, 20)
	vals, cols := chart.Widen(offset(raw, floor), cols)

	out := m.plot(vals, cols, chartHeight, floor, top)
	if mark := chart.Marks(errs, cols, '^'); strings.TrimSpace(mark) != "" {
		out = append(out, m.paint(m.tbl.Char[2], axisPad+mark))
	}
	return out
}
