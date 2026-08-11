package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/dawsonxiong/thock/internal/chart"
	"github.com/dawsonxiong/thock/internal/history"
	"github.com/dawsonxiong/thock/internal/layout"
)

const (
	// maWindow smooths the wpm line over enough runs to hide one bad morning
	// without hiding a month of practice.
	maWindow = 5
	// maxGroupRows caps the setup table: past a handful it stops being a
	// comparison and starts being a list.
	maxGroupRows = 6
	// wpmSpan is the smallest wpm range a chart will zoom to, so a steady week
	// is not magnified into drama.
	wpmSpan = 20
)

// filters lists the selectable views: every recorded setup, most practised
// first, behind an empty key meaning all of them pooled.
func (m *Model) filters() []string {
	groups := history.Summarise(m.records)
	out := make([]string, 1, len(groups)+1)
	for _, g := range groups {
		out = append(out, g.Key)
	}
	return out
}

func (m *Model) cycleFilter(delta int) {
	n := len(m.filters())
	if n == 0 {
		return
	}
	m.statsFilter = (m.statsFilter + delta + n) % n
}

func (m *Model) statsRows(box int) []string {
	filters := m.filters()
	if m.statsFilter >= len(filters) {
		m.statsFilter = 0
	}
	key := filters[m.statsFilter]

	title := "stats"
	if key != "" {
		title += " · " + key
	}
	rows := []string{
		m.spread(box, title, "←→ filter"),
		m.paint(m.tbl.Dim, strings.Repeat("─", box)),
		"",
	}
	hints := m.paint(m.tbl.Dim, "esc back · ←→ filter · ctrl+c quit")

	s := history.Track(m.records, key)
	if len(s.WPM) == 0 {
		return append(rows,
			m.paint(m.tbl.Dim, "no results yet — press esc and start typing"), "", hints)
	}

	best, avg, acc, cons := s.Summary()
	figures := [][2]string{
		{fmt.Sprintf("%.0f", best), "best"},
		{fmt.Sprintf("%.0f", avg), "avg"},
		{fmt.Sprintf("%.0f%%", acc), "acc"},
		{fmt.Sprintf("%.0f%%", cons), "cons"},
		{fmt.Sprintf("%d", len(s.WPM)), "runs"},
	}
	// Figures are dropped from the right until the row fits, rather than being
	// allowed to run past the edge of the box.
	for len(figures) > 1 && headlineWidth(figures) > box {
		figures = figures[:len(figures)-1]
	}
	rows = append(rows, m.headline(figures), "")

	// Sections shrink before they vanish, so a short terminal loses resolution
	// rather than losing the screen.
	height, full := chartHeight, true
	switch {
	case m.h < 20:
		height, full = 3, false
	case m.h < 26:
		height = 4
	}

	if plot := m.wpmPlot(box, height, full, s); len(plot) > 0 {
		rows = append(rows, plot...)
		rows = append(rows, "")
	}
	if full {
		if tracks := m.trackRows(box, s); len(tracks) > 0 {
			rows = append(rows, tracks...)
			rows = append(rows, "")
		}
	}
	// Two rows are held back for the blank line and the hints below the table.
	rows = append(rows, m.groupTable(key, m.h-2-len(rows)-2)...)
	return append(rows, "", hints)
}

// wpmPlot draws one bar per run, with a smoothed line beneath it: the bars show
// what happened, the line shows which way it is going. The line is a sparkline,
// so it carries shape but not scale — the figures on the section header are what
// say how large the movement actually is.
func (m *Model) wpmPlot(box, height int, full bool, s history.Series) []string {
	cols := plotWidth(box)
	if cols == 0 || len(s.WPM) < 3 {
		return nil
	}
	early, late := history.Trend(s.WPM)
	note := ""
	if early > 0 {
		arrow := "↑"
		if late < early {
			arrow = "↓"
		}
		note = fmt.Sprintf("%.0f → %.0f   %s %.0f", early, late, arrow, math.Abs(late-early))
	}

	floor, top := chart.Frame(s.WPM, wpmSpan)
	vals, cols := chart.Widen(offset(s.WPM, floor), cols)

	out := append([]string{m.spread(box, "wpm", note)}, m.plot(vals, cols, height, floor, top)...)
	if full {
		ma := history.MovingAvg(s.WPM, maWindow)
		out = append(out, m.paint(m.tbl.Dim, trackLabel("avg"))+
			m.paint(m.tbl.Text, chart.Sparkline(ma, cols)))
	}
	return out
}

// trackRows give accuracy and consistency their own lines: speed bought with
// more mistakes is not an improvement, and the wpm chart alone cannot show that.
//
// A sparkline stretches its eight levels across whatever range the data has, so
// each row is labelled with its own bounds. Accuracy lives in a handful of
// points near the top of its scale, and unlabelled that narrow a band would
// read as violent swings.
func (m *Model) trackRows(box int, s history.Series) []string {
	cols := plotWidth(box)
	if cols == 0 || len(s.Accuracy) < 3 {
		return nil
	}
	row := func(label string, v []float64) string {
		lo, hi := span(v)
		note := fmt.Sprintf("%.0f–%.0f%%", lo, hi)
		if hi-lo < 1 {
			note = fmt.Sprintf("%.0f%%", hi)
		}
		return m.paint(m.tbl.Dim, trackLabel(label)) +
			m.paint(m.tbl.Text, chart.Sparkline(v, cols-8)) +
			m.paint(m.tbl.Dim, fmt.Sprintf("%8s", note))
	}
	return []string{row("acc", s.Accuracy), row("cons", s.Consistency)}
}

// groupTable compares the setups actually practised, using the same
// comparability keys as the personal bests so a 30 second run is never averaged
// against a 120 second one. budget is how many rows the table may occupy.
func (m *Model) groupTable(key string, budget int) []string {
	groups := history.Summarise(m.records)
	if len(groups) == 0 || budget < 2 {
		return nil
	}
	n := budget - 1 // the header row
	if n > maxGroupRows {
		n = maxGroupRows
	}
	if len(groups) > n {
		groups = groups[:n]
	}

	out := []string{m.paint(m.tbl.Dim,
		fmt.Sprintf("  %-6s%6s%6s%7s%6s", "", "best", "avg", "acc", "runs"))}
	for _, g := range groups {
		marker, name := "  ", m.tbl.Dim
		if g.Key == key {
			marker, name = m.paint(m.tbl.Accent, "❯ "), m.tbl.Text
		}
		out = append(out, marker+
			m.paint(name, fmt.Sprintf("%-6s", g.Key))+
			m.paint(m.tbl.Accent, fmt.Sprintf("%6.0f", g.Best))+
			m.paint(m.tbl.Dim, fmt.Sprintf("%6.0f%6.0f%%%6d", g.WPM, g.Accuracy, g.Runs)))
	}
	return out
}

// headline renders the figures above the chart as a row, each value in the
// accent with its label muted so the numbers read before the words.
func (m *Model) headline(pairs [][2]string) string {
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteString(m.paint(m.tbl.Dim, "   ·   "))
		}
		b.WriteString(m.paint(m.tbl.Accent, p[0]))
		b.WriteByte(' ')
		b.WriteString(m.paint(m.tbl.Dim, p[1]))
	}
	return b.String()
}

// trackLabel names a sparkline row inside the same width as the plot's axis
// column, so the two line up. The trailing blank is what keeps a four-letter
// label off the first block of its line.
func trackLabel(s string) string { return fmt.Sprintf(" %-5s", s) }

// headlineWidth is the cell width the figures will occupy, separator included.
func headlineWidth(pairs [][2]string) int {
	n := 7 * (len(pairs) - 1)
	for _, p := range pairs {
		n += layout.Width(p[0]) + 1 + layout.Width(p[1])
	}
	return n
}

// span is the smallest and largest value in v, which is the range a sparkline
// stretches its levels across.
func span(v []float64) (lo, hi float64) {
	if len(v) == 0 {
		return 0, 0
	}
	lo, hi = v[0], v[0]
	for _, x := range v {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	return lo, hi
}

// spread puts a label on the left and a note on the right of one row, with the
// note flush to the edge of the box.
func (m *Model) spread(box int, left, right string) string {
	gap := box - layout.Width(left) - layout.Width(right)
	if gap < 2 {
		gap = 2
	}
	return m.paint(m.tbl.Text, left) + strings.Repeat(" ", gap) + m.paint(m.tbl.Dim, right)
}
