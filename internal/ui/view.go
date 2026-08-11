package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dawsonxiong/thock/internal/layout"
)

const (
	maxBox = 76
	minBox = 24
)

// banner is the wordmark, shown only before the first keystroke: once typing
// starts the screen gives everything it has to the text. Generated once with
// "figlet -f rounded thock" and embedded, so there is no runtime dependency
// and no font file to ship.
var banner = []string{
	`       _                 _     `,
	`   _  | |               | |    `,
	` _| |_| |__   ___   ____| |  _ `,
	`(_   _)  _ \ / _ \ / ___) |_/ )`,
	`  | |_| | | | |_| ( (___|  _ ( `,
	`   \__)_| |_|\___/ \____)_| \_)`,
}

// bannerWidth is the wordmark's width in cells. Every row is padded to it, so
// the banner can be skipped wholesale when the viewport is narrower.
const bannerWidth = 31

func (m *Model) boxWidth() int {
	w := m.w - 6
	if w > maxBox {
		w = maxBox
	}
	return w
}

// render builds the frame and records where the caret should sit.
func (m *Model) render() string {
	m.caretX, m.caretY, m.caretOK = 0, 0, false

	if m.err != nil {
		return "\n  " + m.paint(m.tbl.Char[2], "error: "+m.err.Error()) + "\n"
	}
	if m.w < minBox+6 || m.h < 12 {
		return "\n  " + m.paint(m.tbl.Dim, "terminal too small") + "\n"
	}

	box := m.boxWidth()
	var rows []string
	switch m.screen {
	case screenResults:
		rows = m.resultRows(box)
	case screenStats:
		rows = m.statsRows(box)
	default:
		rows = m.testRows(box)
	}

	pad := strings.Repeat(" ", (m.w-box)/2)
	top := (m.h - len(rows)) / 2
	if top < 1 {
		top = 1
	}
	m.caretY += top

	var b strings.Builder
	b.Grow(m.w * len(rows) * 2)
	for i := 0; i < top; i++ {
		b.WriteByte('\n')
	}
	for i, r := range rows {
		b.WriteString(pad)
		b.WriteString(r)
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	m.caretX += len(pad)
	return b.String()
}

// paint wraps text in a prefix from the resolved theme table.
func (m *Model) paint(prefix, s string) string {
	if prefix == "" {
		return s
	}
	return prefix + s + m.tbl.Reset
}

// choices renders a label followed by its options, marking the active one.
func (m *Model) choices(label string, values []string, active int) string {
	var b strings.Builder
	b.WriteString(m.paint(m.tbl.Dim, label))
	for i, v := range values {
		b.WriteByte(' ')
		if i == active {
			b.WriteString(m.paint(m.tbl.Accent, v))
		} else {
			b.WriteString(m.paint(m.tbl.Dim, v))
		}
	}
	return b.String()
}

func (m *Model) testRows(box int) []string {
	var rows []string

	// The wordmark is the first thing to go when space is short: it is
	// decoration, and the text it would push off screen is the point.
	if !m.running && len(m.eng.Log) == 0 && box >= bannerWidth && m.h >= 18 {
		for _, l := range banner {
			rows = append(rows, m.paint(m.tbl.Accent, l))
		}
		rows = append(rows, "")
	}

	rows = append(rows, m.configBar())
	rows = append(rows, m.paint(m.tbl.Dim, strings.Repeat("─", box)))
	rows = append(rows, "")

	if m.overlay == overlayOptions {
		rows = append(rows, m.optionRowsView()...)
		rows = append(rows, "")
	}

	// The wrap is a pure function of the words and the width, so it is simply
	// recomputed whenever either changes rather than kept across a resize.
	if m.lines == nil || m.wrapVer != m.eng.Version() {
		m.lines = layout.Wrap(m.eng, box)
		m.wrapVer = m.eng.Version()
	}
	active := layout.LineOf(m.lines, m.eng.WordIdx)
	top, bottom := layout.Window(m.lines, active)

	m.cache.Sync(box, m.eng.Version(), m.opts.Theme, len(m.lines), active)
	textStart := len(rows)
	for i := top; i < bottom; i++ {
		rows = append(rows, m.cache.Get(m.eng, m.lines, &m.tbl, i))
	}
	for i := bottom - top; i < layout.VisibleLines; i++ {
		rows = append(rows, "")
	}

	if m.overlay == overlayNone && active >= top && active < bottom {
		m.caretX = layout.CaretColumn(m.eng, m.lines[active])
		m.caretY = textStart + (active - top)
		m.caretOK = true
	}

	rows = append(rows, "")
	rows = append(rows, m.statusBar(box))
	return rows
}

func (m *Model) configBar() string {
	var parts []string
	lists := []string{"1k", "5k"}
	switch m.opts.Mode {
	case ModeTime:
		parts = append(parts,
			m.choices("time", durations, indexOf(durations, strconv.Itoa(m.opts.Duration))),
			m.choices("list", lists, indexOf(lists, string(m.opts.List))))
	case ModeWords:
		parts = append(parts,
			m.choices("words", wordCounts, indexOf(wordCounts, strconv.Itoa(m.opts.Words))),
			m.choices("list", lists, indexOf(lists, string(m.opts.List))))
	default:
		lens := []string{"any", "short", "medium", "long"}
		parts = append(parts, m.choices("quote", lens, indexOf(lens, string(m.opts.Length))))
	}
	return strings.Join(parts, m.paint(m.tbl.Dim, "   "))
}

// statusBar shows the clock or progress on the left and the key hints on the
// right, with the gap sized so the hints sit flush to the box edge.
func (m *Model) statusBar(box int) string {
	var left, leftPlain string
	if m.opts.Mode == ModeTime {
		remaining := m.opts.Duration - int(m.elapsed.Seconds())
		if remaining < 0 {
			remaining = 0
		}
		leftPlain = strconv.Itoa(remaining)
	} else {
		leftPlain = fmt.Sprintf("%d/%d", m.eng.WordIdx, len(m.eng.Words))
	}
	left = m.paint(m.tbl.Accent, leftPlain)

	if m.running {
		wpm := fmt.Sprintf("  %.0f wpm", m.live.WPM)
		left += m.paint(m.tbl.Dim, wpm)
		leftPlain += wpm
	}

	hint := "esc restart · tab options · ctrl+c quit"
	switch {
	case m.overlay == overlayOptions:
		hint = "↑↓ row · ←→ change · enter restart · esc back"
	// Stats are only offered when they can be reached: opening them mid-test
	// would throw the run away.
	case !m.running:
		hint = "esc restart · tab options · ctrl+s stats · ctrl+c quit"
	}
	gap := box - layout.Width(leftPlain) - layout.Width(hint)
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + m.paint(m.tbl.Dim, hint)
}

// optionRowsView renders the options pane, which sits between the config bar
// and the text so the test stays visible while settings change.
func (m *Model) optionRowsView() []string {
	rows := m.optionRows()
	out := make([]string, 0, len(rows))
	for i, r := range rows {
		marker := "  "
		if i == m.optRow {
			marker = m.paint(m.tbl.Accent, "❯ ")
		}
		label := m.paint(m.tbl.Dim, fmt.Sprintf("%-7s", r.label))
		out = append(out, marker+label+valueList(m, r))
	}
	return out
}

func valueList(m *Model, r optRow) string {
	var b strings.Builder
	for i, v := range r.values {
		if i > 0 {
			b.WriteByte(' ')
		}
		if i == r.idx {
			b.WriteString(m.paint(m.tbl.Accent, v))
		} else {
			b.WriteString(m.paint(m.tbl.Dim, v))
		}
	}
	return b.String()
}

// caret reports the cursor position for this frame.
func (m *Model) caret() (int, int, bool) { return m.caretX, m.caretY, m.caretOK }
