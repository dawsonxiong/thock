// Package theme holds the colour palettes as plain data and resolves them into
// a table of raw ANSI prefixes. Building the escape sequences once, up front,
// is what keeps the per-keystroke render path free of styling work.
package theme

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dawsonxiong/thock/internal/typing"
)

// Colour is either a hex string ("#rrggbb"), an ANSI palette reference
// ("ansi:4"), or empty for the terminal's default foreground.
type Colour string

// Style is one visual role: a colour plus attributes. Attributes are stored
// separately from colour so that stripping colour still leaves the interface
// readable.
type Style struct {
	FG        Colour
	Bold      bool
	Faint     bool
	Underline bool
}

// Theme is a full palette.
type Theme struct {
	Name string
	Desc string

	Pending   Style // not yet typed
	Correct   Style // typed correctly
	Incorrect Style // typed wrongly
	Extra     Style // typed past the end of a word
	Missed    Style // skipped by an early space

	Text   Style // ordinary interface text
	Dim    Style // metadata, hints, inactive options
	Accent Style // live figures, active option, headline

	// Racers colour the other players in a race, in order of arrival. None
	// of them is the accent, which always means you, or the red that means
	// a mistake.
	Racers [6]Colour
}

// Table is a resolved theme: ready-to-write escape sequences indexed by the
// state they paint.
type Table struct {
	Char   [5]string // indexed by typing.CharState
	Text   string
	Dim    string
	Accent string
	Reset  string

	// Racer paints another player's name and lane, and Ghost their caret in
	// your text: the character they are on, drawn as a block of their colour.
	Racer [6]string
	Ghost [6]string
}

const reset = "\x1b[0m"

// Resolve turns a theme into escape sequences. When colour is false the hues
// are dropped but the attributes are kept and strengthened, so correct, wrong
// and pending text stay distinguishable in a NO_COLOR environment.
func Resolve(t Theme, colour bool) Table {
	tb := Table{Reset: reset}
	if !colour {
		t = monochrome(t)
	}
	tb.Char[typing.CharPending] = sgr(t.Pending, colour)
	tb.Char[typing.CharCorrect] = sgr(t.Correct, colour)
	tb.Char[typing.CharIncorrect] = sgr(t.Incorrect, colour)
	tb.Char[typing.CharExtra] = sgr(t.Extra, colour)
	tb.Char[typing.CharMissed] = sgr(t.Missed, colour)
	tb.Text = sgr(t.Text, colour)
	tb.Dim = sgr(t.Dim, colour)
	tb.Accent = sgr(t.Accent, colour)
	for i, c := range t.Racers {
		tb.Racer[i] = sgr(Style{FG: c}, colour)
		// Reverse video turns the colour into the cell's background, which
		// reads as a caret on any terminal background, and stays a caret with
		// the colour stripped.
		tb.Ghost[i] = "\x1b[7m"
		if p := sgr(Style{FG: c}, colour); p != "" {
			tb.Ghost[i] = "\x1b[7;" + p[2:]
		}
	}
	return tb
}

// monochrome re-expresses a palette using attributes alone.
func monochrome(t Theme) Theme {
	t.Pending = Style{Faint: true}
	t.Correct = Style{}
	t.Incorrect = Style{Underline: true, Bold: true}
	t.Extra = Style{Underline: true, Faint: true}
	t.Missed = Style{Underline: true}
	t.Text = Style{}
	t.Dim = Style{Faint: true}
	t.Accent = Style{Bold: true}
	return t
}

// sgr builds a single combined escape sequence for a style.
func sgr(s Style, colour bool) string {
	var p []string
	if s.Bold {
		p = append(p, "1")
	}
	if s.Faint {
		p = append(p, "2")
	}
	if s.Underline {
		p = append(p, "4")
	}
	if colour {
		if c := colourParams(s.FG); c != "" {
			p = append(p, c)
		}
	}
	if len(p) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(p, ";") + "m"
}

func colourParams(c Colour) string {
	s := string(c)
	switch {
	case s == "":
		return ""
	case strings.HasPrefix(s, "ansi:"):
		n, err := strconv.Atoi(s[5:])
		if err != nil || n < 0 || n > 15 {
			return ""
		}
		if n < 8 {
			return strconv.Itoa(30 + n)
		}
		return strconv.Itoa(90 + n - 8)
	case strings.HasPrefix(s, "#") && len(s) == 7:
		r, err1 := strconv.ParseUint(s[1:3], 16, 8)
		g, err2 := strconv.ParseUint(s[3:5], 16, 8)
		b, err3 := strconv.ParseUint(s[5:7], 16, 8)
		if err1 != nil || err2 != nil || err3 != nil {
			return ""
		}
		return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
	}
	return ""
}

// Get returns a theme by name.
func Get(name string) (Theme, bool) {
	t, ok := registry[name]
	return t, ok
}

// Names lists every theme name, with the default first and the rest sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		if n != Default {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return append([]string{Default}, out...)
}
