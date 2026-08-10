package stats

import (
	"math"
	"testing"
	"time"

	"github.com/dawsonxiong/thock/internal/typing"
)

func typeAll(e *typing.Engine, s string, start time.Duration) time.Duration {
	at := start
	for _, r := range s {
		at += 100 * time.Millisecond
		if r == ' ' {
			e.Space(at)
			continue
		}
		e.TypeRune(r, at)
	}
	return at
}

// Accuracy is defined over keystroke history, not the final text. Correcting a
// mistake leaves the text perfect but must still cost accuracy, and the
// backspace itself must not count as a second mistake.
func TestAccuracyIsKeypressHistoryNotFinalText(t *testing.T) {
	e := typing.New([]string{"a"})
	e.TypeRune('x', 100*time.Millisecond) // wrong
	e.Backspace(200 * time.Millisecond)   // corrected
	e.TypeRune('a', 300*time.Millisecond) // right

	got := Compute(e, time.Minute)

	// Two character keystrokes, one correct.
	if math.Abs(got.Accuracy-50) > 1e-9 {
		t.Errorf("accuracy = %v, want 50\n"+
			"  100 would mean it scored the final text instead of the keystrokes\n"+
			"  33.3 would mean the backspace leaked into the denominator", got.Accuracy)
	}
	// The final text is correct, so the character itself still scores.
	if got.CorrectChars != 1 {
		t.Errorf("CorrectChars = %d, want 1", got.CorrectChars)
	}
	if got.IncorrectChars != 0 {
		t.Errorf("IncorrectChars = %d, want 0 (the x was deleted)", got.IncorrectChars)
	}
}

// Consistency must reproduce Monkeytype's kogasa curve exactly, including the
// cubic and quintic terms and the population standard deviation.
func TestConsistencyKogasaExact(t *testing.T) {
	t.Run("perfectly even typing scores 100", func(t *testing.T) {
		got := consistency([]Sample{{Raw: 60}, {Raw: 60}, {Raw: 60}, {Raw: 60}})
		if math.Abs(got-100) > 1e-9 {
			t.Errorf("consistency = %v, want 100", got)
		}
	})

	t.Run("uneven typing matches the kogasa series", func(t *testing.T) {
		// mean 60, population sd 20, so cov = 1/3.
		// x    = 1/3 + (1/3)^3/3 + (1/3)^5/5 = 0.34650206
		// score = 100 * (1 - tanh(x))       = 66.673101
		got := consistency([]Sample{{Raw: 40}, {Raw: 80}})
		if math.Abs(got-66.673101) > 1e-3 {
			t.Errorf("consistency = %v, want 66.673101\n"+
				"  67.848726 would mean the x^3 and x^5 terms are missing\n"+
				"  52.928700 would mean it used the sample sd instead of the population sd", got)
		}
	})

	t.Run("a single sample is 100, not NaN", func(t *testing.T) {
		got := consistency([]Sample{{Raw: 60}})
		if math.IsNaN(got) || math.Abs(got-100) > 1e-9 {
			t.Errorf("consistency = %v, want 100", got)
		}
	})
}

// Skipping the rest of a word with an early space costs speed but not accuracy:
// no key was ever pressed for the missed characters. The space itself is an
// error, because the word before it was not perfect.
func TestMissedCharsHurtWPMNotAccuracy(t *testing.T) {
	e := typing.New([]string{"hello", "world"})
	typeAll(e, "he world", 0)

	got := Compute(e, time.Minute)

	if got.CorrectChars != 7 {
		t.Errorf("CorrectChars = %d, want 7 (he + world)", got.CorrectChars)
	}
	if got.MissedChars != 3 {
		t.Errorf("MissedChars = %d, want 3 (llo)", got.MissedChars)
	}
	if got.IncorrectChars != 0 {
		t.Errorf("IncorrectChars = %d, want 0 (missed is not incorrect)", got.IncorrectChars)
	}
	// 8 keystrokes: h e <space> w o r l d. Only the space was wrong, because
	// "he" is not "hello", so no correct space is credited either.
	if want := 7.0 / 8 * 100; math.Abs(got.Accuracy-want) > 1e-9 {
		t.Errorf("accuracy = %v, want %v", got.Accuracy, want)
	}
	// (7 correct chars + 0 correct spaces) / 5 over one minute.
	if math.Abs(got.WPM-1.4) > 1e-9 {
		t.Errorf("wpm = %v, want 1.4", got.WPM)
	}
	// Nothing extra was typed, so raw matches.
	if math.Abs(got.Raw-got.WPM) > 1e-9 {
		t.Errorf("raw = %v, want it to equal wpm %v", got.Raw, got.WPM)
	}

	// Typing past the end of a word adds to raw but never to wpm.
	e2 := typing.New([]string{"hello", "world"})
	typeAll(e2, "he worldx", 0)
	got2 := Compute(e2, time.Minute)

	if got2.ExtraChars != 1 {
		t.Errorf("ExtraChars = %d, want 1", got2.ExtraChars)
	}
	if got2.Raw <= got2.WPM {
		t.Errorf("raw = %v, want it above wpm %v once an extra char is typed", got2.Raw, got2.WPM)
	}
	if got2.Accuracy >= 100 {
		t.Errorf("accuracy = %v, want below 100", got2.Accuracy)
	}
}
