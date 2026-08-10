// Package layout turns a list of words into wrapped lines and picks the
// visible window. Everything here is a pure function of the words and the
// viewport width, so there is no stored geometry to go stale on a resize.
package layout

import (
	"github.com/dawsonxiong/thock/internal/typing"
)

// Line is an inclusive range of word indices sharing a rendered row.
type Line struct {
	First, Last int
}

// VisibleLines is how many rows of text the test area shows at once.
const VisibleLines = 3

// Wrap greedily breaks words into rows no wider than width. Word widths
// include any extra characters typed past the target, so overtyping pushes
// the line exactly as it does on Monkeytype.
func Wrap(e *typing.Engine, width int) []Line {
	if width <= 0 || len(e.Words) == 0 {
		return nil
	}
	var lines []Line
	first := 0
	cur := 0

	for i := range e.Words {
		w := e.Words[i].Width()
		next := w
		if i > first {
			next++ // the separating space
		}
		if i > first && cur+next > width {
			lines = append(lines, Line{First: first, Last: i - 1})
			first = i
			cur = w
			continue
		}
		cur += next
	}
	lines = append(lines, Line{First: first, Last: len(e.Words) - 1})
	return lines
}

// LineOf returns the index of the line holding the given word.
func LineOf(lines []Line, wordIdx int) int {
	for i, l := range lines {
		if wordIdx >= l.First && wordIdx <= l.Last {
			return i
		}
	}
	if len(lines) == 0 {
		return 0
	}
	return len(lines) - 1
}

// Window picks the first visible line, keeping the active line in the middle
// so there is one row of context above and one row of lookahead below. It is
// derived from the active word every frame, so scrolling cannot drift.
func Window(lines []Line, activeLine int) (top, bottom int) {
	top = activeLine - 1
	if top < 0 {
		top = 0
	}
	if max := len(lines) - VisibleLines; top > max {
		top = max
	}
	if top < 0 {
		top = 0
	}
	bottom = top + VisibleLines
	if bottom > len(lines) {
		bottom = len(lines)
	}
	return top, bottom
}

// CaretColumn is the display column of the caret within its line, measured in
// terminal cells so wide glyphs land correctly.
func CaretColumn(e *typing.Engine, line Line) int {
	col := 0
	for i := line.First; i < e.WordIdx && i <= line.Last; i++ {
		col += e.Words[i].Width() + 1
	}
	if e.WordIdx > line.Last {
		return col
	}
	w := e.Current()
	if w == nil {
		return col
	}
	return col + typedWidth(w)
}

// typedWidth is the rendered width of what has been typed for a word, which
// is the target's width for the matched prefix plus the width of any extras.
func typedWidth(w *typing.Word) int {
	n := 0
	for i := range w.Typed {
		if i < len(w.Target) {
			n += runeWidth(w.Target[i])
		} else {
			n += runeWidth(w.Typed[i])
		}
	}
	return n
}
