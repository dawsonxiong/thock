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

	// Figures in a grid: the pair that defines the run in the accent, and the
	// pair that qualifies it muted underneath, in the same columns. Raw sits
	// below wpm and consistency below accuracy because each is the other's
	// caveat — speed before mistakes, and how evenly that speed was held.
	pb := ""
	if m.isPB {
		pb = "  " + m.paint(m.tbl.Accent, "✦ pb")
	}
	rows = append(rows,
		m.figure(m.tbl.Accent, fmt.Sprintf("%.0f", r.WPM), "wpm")+
			m.figure(m.tbl.Accent, fmt.Sprintf("%.0f%%", r.Accuracy), "acc")+pb,
		m.figure(m.tbl.Dim, fmt.Sprintf("%.0f", r.Raw), "raw")+
			m.figure(m.tbl.Dim, fmt.Sprintf("%.0f%%", r.Consistency), "cons"),
		"")

	rows = append(rows, m.chartRows(box)...)
	rows = append(rows, "")
	rows = append(rows, m.paint(m.tbl.Dim, m.charCounts(box)))

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
	rows = append(rows, m.paint(m.tbl.Dim, "test    "+setup))

	if m.opts.Mode == ModeQuotes && m.quote.Source != "" {
		rows = append(rows, "", m.paint(m.tbl.Text, "— "+m.quote.Attribution()))
	}

	rows = append(rows, "", m.paint(m.tbl.Dim, fit(box,
		"enter next · esc repeat · tab options · ctrl+s stats · ctrl+c quit",
		"enter next · esc repeat · tab options · ctrl+s stats",
		"enter next · esc repeat · ctrl+s stats")))
	return rows
}

// figure lays out one statistic in fixed columns — the value right-aligned, the
// label padded — so figures line up in a grid however many digits they have.
func (m *Model) figure(style, value, label string) string {
	pad := 6 - layout.Width(value)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + m.paint(style, value) +
		"   " + m.paint(m.tbl.Dim, fmt.Sprintf("%-6s", label))
}

// charCounts names the four character tallies. Four bare numbers separated by
// slashes are only readable to someone who already knows the order, which is
// nobody reading their own result for the first time. The compact form is kept
// for boxes too narrow to spell them out.
func (m *Model) charCounts(box int) string {
	r := m.res
	long := fmt.Sprintf("chars   %d correct · %d wrong · %d extra · %d missed",
		r.CorrectChars, r.IncorrectChars, r.ExtraChars, r.MissedChars)
	if layout.Width(long) <= box {
		return long
	}
	return fmt.Sprintf("chars   %d/%d/%d/%d",
		r.CorrectChars, r.IncorrectChars, r.ExtraChars, r.MissedChars)
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

	// Neither the plot nor the carets under it are guessable from their shape,
	// so both are named. The legend only appears when there is a caret to
	// explain.
	note := ""
	if mark := chart.Marks(errs, cols, '^'); strings.TrimSpace(mark) != "" {
		out = append(out, m.paint(m.tbl.Char[2], axisPad+mark))
		note = "^ mistake"
	}
	return append([]string{m.spread(box, "raw wpm per second", note)}, out...)
}
