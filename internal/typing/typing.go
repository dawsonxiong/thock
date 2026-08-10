// Package typing implements the test state machine: target words, what the
// user typed, and a timestamped log of every keystroke. It is deliberately
// free of any terminal or clock dependency so it can be exercised directly.
package typing

import (
	"time"

	"github.com/mattn/go-runewidth"
)

// CharState is the render state of a single character position.
type CharState uint8

const (
	CharPending   CharState = iota // not reached yet
	CharCorrect                    // typed, matches target
	CharIncorrect                  // typed, does not match target
	CharExtra                      // typed past the end of the target word
	CharMissed                     // skipped by an early space, never typed
)

// KeyKind classifies a logged keystroke. The distinction matters because
// deletions are excluded from the accuracy denominator.
type KeyKind uint8

const (
	KeyChar KeyKind = iota
	KeySpace
	KeyBackspace
	KeyDeleteWord
)

// Keystroke is one keypress. Correct is evaluated at press time and never
// recomputed, because accuracy is defined over keystroke history rather than
// over the final state of the text.
type Keystroke struct {
	At      time.Duration
	R       rune
	Kind    KeyKind
	Correct bool
}

// maxExtra caps how far past a word's length the user can keep typing, so a
// stuck key cannot grow a word without bound.
const maxExtra = 20

// Word pairs a target with what has been typed against it.
type Word struct {
	Target []rune
	Typed  []rune
}

// Width is the current rendered width of the word, including any extra
// characters typed past the target.
func (w *Word) Width() int {
	n := 0
	for _, r := range w.Target {
		n += runewidth.RuneWidth(r)
	}
	for i := len(w.Target); i < len(w.Typed); i++ {
		n += runewidth.RuneWidth(w.Typed[i])
	}
	return n
}

// Correct reports whether the word has been typed exactly.
func (w *Word) Correct() bool {
	if len(w.Typed) != len(w.Target) {
		return false
	}
	for i, r := range w.Target {
		if w.Typed[i] != r {
			return false
		}
	}
	return true
}

// Started reports whether any input has been entered for this word.
func (w *Word) Started() bool { return len(w.Typed) > 0 }

// Engine holds the whole test. It owns no clock: callers pass the elapsed
// time with each event, which keeps every behaviour deterministically testable.
type Engine struct {
	Words   []Word
	WordIdx int
	Log     []Keystroke

	// version changes only when a word's rendered width changes, so the
	// layout cache can skip re-wrapping on ordinary keystrokes.
	version uint64
}

// New builds an engine over the given target words.
func New(words []string) *Engine {
	e := &Engine{
		Words: make([]Word, len(words)),
		Log:   make([]Keystroke, 0, 4096),
	}
	for i, w := range words {
		e.Words[i] = Word{Target: []rune(w)}
	}
	return e
}

// Version identifies the current layout-affecting state of the words.
func (e *Engine) Version() uint64 { return e.version }

// Current returns the word being typed, or nil once the test is exhausted.
func (e *Engine) Current() *Word {
	if e.WordIdx >= len(e.Words) {
		return nil
	}
	return &e.Words[e.WordIdx]
}

// Done reports whether every word has been passed. Only word-count and quote
// tests can reach this; timed tests end on the clock.
func (e *Engine) Done() bool { return e.WordIdx >= len(e.Words) }

// TypeRune applies a printable character to the current word.
func (e *Engine) TypeRune(r rune, at time.Duration) {
	w := e.Current()
	if w == nil {
		return
	}
	if len(w.Typed) >= len(w.Target)+maxExtra {
		return
	}

	correct := len(w.Typed) < len(w.Target) && w.Target[len(w.Typed)] == r
	if len(w.Typed) >= len(w.Target) {
		// Typing past the target grows the word, so layout must recompute.
		e.version++
	}
	w.Typed = append(w.Typed, r)
	e.Log = append(e.Log, Keystroke{At: at, R: r, Kind: KeyChar, Correct: correct})
}

// Space advances to the next word. Anything left untyped in the current word
// is locked in as missed. A space pressed before any input is ignored, matching
// Monkeytype, which does not let a leading space burn a word.
func (e *Engine) Space(at time.Duration) {
	w := e.Current()
	if w == nil || !w.Started() {
		return
	}
	e.Log = append(e.Log, Keystroke{At: at, R: ' ', Kind: KeySpace, Correct: w.Correct()})
	e.WordIdx++
}

// Backspace deletes one character, stepping back into the previous word when
// the current one is empty. Stepping back is only permitted onto a word that
// was typed imperfectly, so a clean run cannot be rewound.
func (e *Engine) Backspace(at time.Duration) {
	w := e.Current()
	if w != nil && w.Started() {
		if len(w.Typed) > len(w.Target) {
			e.version++
		}
		w.Typed = w.Typed[:len(w.Typed)-1]
		e.Log = append(e.Log, Keystroke{At: at, Kind: KeyBackspace})
		return
	}
	if e.WordIdx > 0 && !e.Words[e.WordIdx-1].Correct() {
		e.WordIdx--
		e.Log = append(e.Log, Keystroke{At: at, Kind: KeyBackspace})
	}
}

// DeleteWord clears the current word, or steps back and clears the previous
// imperfect one when the current word is already empty.
func (e *Engine) DeleteWord(at time.Duration) {
	w := e.Current()
	if w != nil && w.Started() {
		if len(w.Typed) > len(w.Target) {
			e.version++
		}
		w.Typed = w.Typed[:0]
		e.Log = append(e.Log, Keystroke{At: at, Kind: KeyDeleteWord})
		return
	}
	if e.WordIdx > 0 && !e.Words[e.WordIdx-1].Correct() {
		e.WordIdx--
		prev := &e.Words[e.WordIdx]
		if len(prev.Typed) > len(prev.Target) {
			e.version++
		}
		prev.Typed = prev.Typed[:0]
		e.Log = append(e.Log, Keystroke{At: at, Kind: KeyDeleteWord})
	}
}

// States returns the render state of every character position of a word,
// which is len(Target) entries plus one per extra character typed.
func (e *Engine) States(wordIdx int) []CharState {
	w := &e.Words[wordIdx]
	n := len(w.Target)
	if len(w.Typed) > n {
		n = len(w.Typed)
	}
	out := make([]CharState, n)
	passed := wordIdx < e.WordIdx

	for i := range out {
		switch {
		case i >= len(w.Target):
			out[i] = CharExtra
		case i < len(w.Typed):
			if w.Typed[i] == w.Target[i] {
				out[i] = CharCorrect
			} else {
				out[i] = CharIncorrect
			}
		case passed:
			out[i] = CharMissed
		default:
			out[i] = CharPending
		}
	}
	return out
}

// Runes returns the characters to display for a word: the target, with any
// extra typed characters appended.
func (e *Engine) Runes(wordIdx int) []rune {
	w := &e.Words[wordIdx]
	if len(w.Typed) <= len(w.Target) {
		return w.Target
	}
	out := make([]rune, 0, len(w.Typed))
	out = append(out, w.Target...)
	out = append(out, w.Typed[len(w.Target):]...)
	return out
}
