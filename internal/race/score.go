package race

import (
	"time"

	"github.com/dawsonxiong/thock/internal/stats"
	"github.com/dawsonxiong/thock/internal/typing"
)

// Replay rebuilds a racer's run from their keystrokes, applying every key
// pressed up to and including until. The engine is deterministic, so the same
// keys over the same words always give the same text, and so the same score.
func Replay(words []string, keys Keys, until time.Duration) *typing.Engine {
	e := typing.New(words)
	Advance(e, keys, 0, until)
	return e
}

// Advance applies keys[from:] up to until to an engine and returns the index
// of the first key not yet applied, so a replay can step forward frame by frame
// without starting over.
func Advance(e *typing.Engine, keys Keys, from int, until time.Duration) int {
	i := from
	for ; i < len(keys) && keys[i].At <= until; i++ {
		k := keys[i]
		switch k.Kind {
		case typing.KeyChar:
			e.TypeRune(k.R, k.At)
		case typing.KeySpace:
			e.Space(k.At)
		case typing.KeyBackspace:
			e.Backspace(k.At)
		case typing.KeyDeleteWord:
			e.DeleteWord(k.At)
		}
	}
	return i
}

// Complete reports whether the text is finished: every word passed, or the
// last word filled in, which needs no trailing space. It is the same finish
// line as a solo words or quote test.
func Complete(e *typing.Engine) bool {
	if e.Done() {
		return true
	}
	if e.WordIdx == len(e.Words)-1 {
		w := e.Current()
		return w != nil && len(w.Typed) >= len(w.Target)
	}
	return false
}

// Score replays a racer's keys and scores them. The finish time is the moment
// of the last keystroke, which is the moment the line was crossed; a racer's
// own clock measured it, so network lag plays no part in who won.
func Score(words []string, keys Keys) (res stats.Result, finished bool) {
	var end time.Duration
	if len(keys) > 0 {
		end = keys[len(keys)-1].At
	}
	e := Replay(words, keys, end)
	return stats.Compute(e, end), Complete(e)
}

// Position is where a racer is in the text: the word and character they are
// on, and how much of the whole text that covers. Only the part of the current
// word that lines up with its target counts, so overtyping a word does not
// move anyone forward.
func Position(e *typing.Engine) (word, char int, done float64) {
	total, covered := 0, 0
	for i := range e.Words {
		w := &e.Words[i]
		n := len(w.Target)
		if i < len(e.Words)-1 {
			n++ // the space after it
		}
		total += n
		switch {
		case i < e.WordIdx:
			covered += n
		case i == e.WordIdx:
			covered += min(len(w.Typed), len(w.Target))
		}
	}
	word = e.WordIdx
	if cur := e.Current(); cur != nil {
		char = min(len(cur.Typed), len(cur.Target))
	}
	if total == 0 {
		return word, char, 0
	}
	if Complete(e) {
		return word, char, 1
	}
	return word, char, float64(covered) / float64(total)
}
