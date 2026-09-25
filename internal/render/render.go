// Package render turns engine state into styled text. It writes raw escape
// sequences taken from a resolved theme table rather than calling into a
// styling library per character, because a typing test redraws on every
// keypress and per-rune styling calls are the thing that makes it feel slow.
package render

import (
	"strings"

	"github.com/dawsonxiong/thock/internal/layout"
	"github.com/dawsonxiong/thock/internal/theme"
	"github.com/dawsonxiong/thock/internal/typing"
)

// Line renders one wrapped row. Runs of characters sharing a state emit a
// single escape sequence between them, so a typical row costs a handful of
// writes rather than one per character.
func Line(e *typing.Engine, l layout.Line, tbl *theme.Table, b *strings.Builder) {
	Marked(e, l, tbl, nil, b)
}

// Mark paints one cell of the text in a style of its own: in a race, where
// another racer's caret is. Sep marks the space after the word rather than a
// character in it.
type Mark struct {
	Word, Char int
	Sep        bool
	Style      string
}

// Marked renders a row like Line, with marks drawn over it. A marked cell
// breaks the run it falls in, so it costs two escape sequences and the rest
// of the row is unaffected.
func Marked(e *typing.Engine, l layout.Line, tbl *theme.Table, marks []Mark, b *strings.Builder) {
	const noState = typing.CharState(255)
	cur := noState

	closeRun := func() {
		if cur != noState && tbl.Char[cur] != "" {
			b.WriteString(tbl.Reset)
		}
		cur = noState
	}
	mark := func(word, char int, sep bool) string {
		for _, m := range marks {
			if m.Word == word && m.Char == char && m.Sep == sep {
				return m.Style
			}
		}
		return ""
	}

	for wi := l.First; wi <= l.Last && wi < len(e.Words); wi++ {
		if wi > l.First {
			closeRun()
			if st := mark(wi-1, 0, true); st != "" {
				b.WriteString(st + " " + tbl.Reset)
			} else {
				b.WriteByte(' ')
			}
		}
		runes := e.Runes(wi)
		states := e.States(wi)
		for i, r := range runes {
			if len(marks) > 0 {
				if st := mark(wi, i, false); st != "" {
					closeRun()
					b.WriteString(st)
					b.WriteRune(r)
					b.WriteString(tbl.Reset)
					continue
				}
			}
			s := states[i]
			if s != cur {
				closeRun()
				if p := tbl.Char[s]; p != "" {
					b.WriteString(p)
				}
				cur = s
			}
			b.WriteRune(r)
		}
	}
	closeRun()
}

// Hidden renders a row with every word blanked to a bar of its own width, so
// the shape of the text shows before the words do. The layout is the same as
// the real text, so nothing moves when it is revealed.
func Hidden(e *typing.Engine, l layout.Line, tbl *theme.Table, b *strings.Builder) {
	b.WriteString(tbl.Char[typing.CharPending])
	for wi := l.First; wi <= l.Last && wi < len(e.Words); wi++ {
		if wi > l.First {
			b.WriteByte(' ')
		}
		b.WriteString(strings.Repeat("▁", e.Words[wi].Width()))
	}
	if tbl.Char[typing.CharPending] != "" {
		b.WriteString(tbl.Reset)
	}
}

// Cache holds rendered rows between frames. A keystroke only ever changes the
// row holding the caret, so the other visible rows are reused verbatim.
type Cache struct {
	lines   []string
	valid   []bool
	width   int
	version uint64
	themeID string
}

// Sync prepares the cache for a frame, dropping everything if the geometry,
// the word widths or the theme changed, and always invalidating the active row.
func (c *Cache) Sync(width int, version uint64, themeID string, nlines, activeLine int) {
	if c.width != width || c.version != version || c.themeID != themeID || len(c.lines) != nlines {
		c.width, c.version, c.themeID = width, version, themeID
		c.lines = make([]string, nlines)
		c.valid = make([]bool, nlines)
	}
	if activeLine >= 0 && activeLine < len(c.valid) {
		c.valid[activeLine] = false
	}
}

// Get returns a rendered row, building it only if it is not already cached.
func (c *Cache) Get(e *typing.Engine, lines []layout.Line, tbl *theme.Table, idx int) string {
	if idx < 0 || idx >= len(c.lines) {
		return ""
	}
	if c.valid[idx] {
		return c.lines[idx]
	}
	var b strings.Builder
	b.Grow(256)
	Line(e, lines[idx], tbl, &b)
	c.lines[idx] = b.String()
	c.valid[idx] = true
	return c.lines[idx]
}

// Invalidate drops every cached row.
func (c *Cache) Invalidate() {
	for i := range c.valid {
		c.valid[i] = false
	}
}
