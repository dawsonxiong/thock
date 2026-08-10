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
	const noState = typing.CharState(255)
	cur := noState

	closeRun := func() {
		if cur != noState && tbl.Char[cur] != "" {
			b.WriteString(tbl.Reset)
		}
		cur = noState
	}

	for wi := l.First; wi <= l.Last && wi < len(e.Words); wi++ {
		if wi > l.First {
			closeRun()
			b.WriteByte(' ')
		}
		runes := e.Runes(wi)
		states := e.States(wi)
		for i, r := range runes {
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
