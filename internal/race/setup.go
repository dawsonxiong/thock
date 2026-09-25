package race

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// Setup is what a round types. A race needs a finish line, so only word-count
// and quote tests can be raced: a timed test has no line to cross first.
type Setup struct {
	Mode   string `json:"mode"`             // words | quotes
	Words  int    `json:"words,omitempty"`  // words mode
	List   string `json:"list,omitempty"`   // words mode: 1k | 5k
	Length string `json:"length,omitempty"` // quotes mode: any | short | medium | long
}

// Race setups.
const (
	ModeWords  = "words"
	ModeQuotes = "quotes"
)

var (
	wordCounts   = []int{10, 25, 50, 100}
	quoteLengths = []string{"any", "short", "medium", "long"}
)

// Presets lists every setup a host can pick, in the order ←→ steps through
// them. The word list stays whatever the host chose.
func Presets(list string) []Setup {
	out := make([]Setup, 0, len(wordCounts)+len(quoteLengths))
	for _, n := range wordCounts {
		out = append(out, Setup{Mode: ModeWords, Words: n, List: list})
	}
	for _, l := range quoteLengths {
		out = append(out, Setup{Mode: ModeQuotes, Length: l})
	}
	return out
}

// Valid reports whether a setup is one of the presets.
func (s Setup) Valid() bool {
	for _, p := range Presets(s.List) {
		if p == s && (s.Mode == ModeQuotes || s.List == "1k" || s.List == "5k") {
			return true
		}
	}
	return false
}

// Label names a setup the way the rest of thock does.
func (s Setup) Label() string {
	if s.Mode == ModeQuotes {
		if s.Length == "" || s.Length == "any" {
			return "quote"
		}
		return "quote · " + s.Length
	}
	return fmt.Sprintf("words %d · %s", s.Words, s.List)
}

// Limits on what a host may send, all well beyond any real round.
const (
	maxWords    = 1000
	maxWordLen  = 64
	maxNameCell = 12
	maxLineCell = 120
)

// CleanName makes a player or room name safe to draw. Names come from other
// machines, and a name is printed straight into the terminal, so anything that
// is not a printable character is dropped: an escape sequence in a name would
// otherwise be run by the reader's terminal.
func CleanName(s string) string {
	s = strings.Join(strings.Fields(printable(s)), " ")
	s = strings.TrimSpace(runewidth.Truncate(s, maxNameCell, ""))
	if s == "" {
		return "racer"
	}
	return s
}

// CleanCode keeps only what a room code can contain: lowercase letters and
// the hyphen between its words.
func CleanCode(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || r == '-' {
			return r
		}
		return -1
	}, strings.ToLower(s))
	if len(s) > 40 {
		return ""
	}
	return s
}

// CleanLine makes a line of text from another machine safe to draw.
func CleanLine(s string) string {
	return runewidth.Truncate(strings.Join(strings.Fields(printable(s)), " "), maxLineCell, "…")
}

func printable(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsPrint(r):
			return r
		}
		return -1
	}, s)
}

// CheckWords vets the text a host sent before it is drawn. A word must be a
// short run of printable, non-space characters; anything else means the host
// is broken or hostile, and the round is refused rather than repaired.
func CheckWords(words []string) error {
	if len(words) == 0 || len(words) > maxWords {
		return fmt.Errorf("a round of %d words is not something thock sends", len(words))
	}
	for _, w := range words {
		rs := []rune(w)
		if len(rs) == 0 || len(rs) > maxWordLen {
			return fmt.Errorf("word %q is not something thock sends", w)
		}
		for _, r := range rs {
			if !unicode.IsPrint(r) || unicode.IsSpace(r) {
				return fmt.Errorf("word %q has a character thock will not draw", w)
			}
		}
	}
	return nil
}

// Ordinal renders a finishing place: 1st, 2nd, 3rd, 4th.
func Ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
