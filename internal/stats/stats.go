// Package stats implements Monkeytype's scoring formulas so that results from
// thock are directly comparable with results from monkeytype.com. It is pure:
// it reads a finished engine and returns numbers.
package stats

import (
	"math"
	"time"

	"github.com/dawsonxiong/thock/internal/typing"
)

// minDuration is the shortest run that yields a meaningful rate. Anything
// briefer is treated as this long so the arithmetic stays finite.
const minDuration = time.Second

// TooShort reports whether a run is too brief to be worth scoring or storing.
// Recording one would write a personal best that could never be beaten.
func TooShort(d time.Duration) bool { return d < minDuration }

// Sample is one second of the test, used to draw the pacing chart and to
// derive consistency.
type Sample struct {
	Second int
	WPM    float64 // cumulative net wpm at the end of this second
	Raw    float64 // raw wpm produced during this second alone
	Errors int     // incorrect keypresses during this second
}

// Result is the full scoring of a completed test.
type Result struct {
	WPM         float64
	Raw         float64
	Accuracy    float64
	Consistency float64

	CorrectChars   int
	IncorrectChars int
	ExtraChars     int
	MissedChars    int

	Duration time.Duration
	Samples  []Sample
}

// Compute scores a test. dur is the elapsed wall time of the test.
//
// WPM counts every correctly entered character plus one space per perfectly
// typed word, divided by five, per minute. Raw counts everything entered.
// Accuracy is measured over keystroke history rather than the final text, so
// a mistake that was corrected still costs accuracy.
func Compute(e *typing.Engine, dur time.Duration) Result {
	// A near-zero duration would divide a real character count by nothing and
	// produce a meaningless rate, so the divisor is floored. Callers are
	// expected to discard runs this short rather than display them.
	secs := math.Max(dur.Seconds(), minDuration.Seconds())
	minutes := secs / 60

	var correctChars, incorrectChars, extraChars, missedChars, typedChars, correctSpaces int

	// Only words the user actually reached count toward the score.
	reached := e.WordIdx
	if reached > len(e.Words) {
		reached = len(e.Words)
	}
	for i := 0; i <= reached && i < len(e.Words); i++ {
		w := &e.Words[i]
		if i == reached && !w.Started() {
			break
		}
		typedChars += len(w.Typed)

		for c := range w.Target {
			switch {
			case c < len(w.Typed) && w.Typed[c] == w.Target[c]:
				correctChars++
			case c < len(w.Typed):
				incorrectChars++
			case i < reached:
				// Passed over by an early space and never typed.
				missedChars++
			}
		}
		if len(w.Typed) > len(w.Target) {
			extraChars += len(w.Typed) - len(w.Target)
		}
		// A space only counts when the word before it was perfect.
		if i < reached && w.Correct() {
			correctSpaces++
		}
	}

	res := Result{
		CorrectChars:   correctChars,
		IncorrectChars: incorrectChars,
		ExtraChars:     extraChars,
		MissedChars:    missedChars,
		Duration:       dur,
	}
	res.WPM = float64(correctChars+correctSpaces) / 5 / minutes
	res.Raw = float64(typedChars+correctSpaces) / 5 / minutes
	res.Accuracy = accuracy(e.Log)
	res.Samples = samples(e.Log, secs, correctChars, correctSpaces)
	res.Consistency = consistency(res.Samples)
	return res
}

// accuracy is the share of character-producing keystrokes that were correct at
// the moment they were pressed. Deletions are excluded from both sides: undoing
// a mistake does not erase it, but it does not count as a second mistake either.
func accuracy(log []typing.Keystroke) float64 {
	var correct, total int
	for _, k := range log {
		if k.Kind != typing.KeyChar && k.Kind != typing.KeySpace {
			continue
		}
		total++
		if k.Correct {
			correct++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(correct) / float64(total) * 100
}

// samples buckets keystrokes into one-second slots. Raw is the rate produced
// within each second; WPM is the cumulative net rate, scaled so the final
// sample agrees with the headline figure.
func samples(log []typing.Keystroke, secs float64, correctChars, correctSpaces int) []Sample {
	n := int(math.Ceil(secs))
	if n <= 0 {
		return nil
	}
	perSec := make([]int, n)
	errPerSec := make([]int, n)
	correctPerSec := make([]int, n)

	for _, k := range log {
		if k.Kind != typing.KeyChar && k.Kind != typing.KeySpace {
			continue
		}
		idx := int(k.At.Seconds())
		if idx >= n {
			idx = n - 1
		}
		if idx < 0 {
			idx = 0
		}
		perSec[idx]++
		if k.Correct {
			correctPerSec[idx]++
		} else {
			errPerSec[idx]++
		}
	}

	// The headline WPM excludes characters that were typed but wrong, so the
	// cumulative curve is scaled to land exactly on it rather than drifting.
	totalCorrect := 0
	for _, c := range correctPerSec {
		totalCorrect += c
	}
	scale := 1.0
	if totalCorrect > 0 {
		scale = float64(correctChars+correctSpaces) / float64(totalCorrect)
	}

	out := make([]Sample, n)
	running := 0
	for i := 0; i < n; i++ {
		running += correctPerSec[i]
		elapsed := float64(i + 1)
		if elapsed > secs {
			elapsed = secs
		}
		out[i] = Sample{
			Second: i + 1,
			Raw:    float64(perSec[i]) / 5 * 60,
			WPM:    float64(running) * scale / 5 / (elapsed / 60),
			Errors: errPerSec[i],
		}
	}
	return out
}

// consistency is kogasa applied to the coefficient of variation of the
// per-second raw rate: steady typing scores high, bursty typing scores low.
func consistency(s []Sample) float64 {
	if len(s) < 2 {
		return 100
	}
	raw := make([]float64, len(s))
	for i, x := range s {
		raw[i] = x.Raw
	}
	mean := 0.0
	for _, v := range raw {
		mean += v
	}
	mean /= float64(len(raw))
	if mean == 0 {
		return 0
	}
	variance := 0.0
	for _, v := range raw {
		variance += (v - mean) * (v - mean)
	}
	// Population standard deviation, matching Monkeytype.
	variance /= float64(len(raw))
	cov := math.Sqrt(variance) / mean
	return kogasa(cov)
}

// kogasa maps a coefficient of variation onto a 0-100 score. The series in the
// exponent is Monkeytype's, and reproducing it exactly is what keeps
// consistency numbers comparable between the two tools.
func kogasa(cov float64) float64 {
	x := cov + cov*cov*cov/3 + math.Pow(cov, 5)/5
	return 100 * (1 - math.Tanh(x))
}
