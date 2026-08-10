package layout

import "github.com/mattn/go-runewidth"

// runeWidth is the terminal cell count for a rune. Every column calculation in
// thock goes through this rather than counting runes, so wide glyphs place the
// caret correctly and CJK text will lay out without changing the engine.
func runeWidth(r rune) int { return runewidth.RuneWidth(r) }

// Width is the cell width of a string.
func Width(s string) int { return runewidth.StringWidth(s) }
