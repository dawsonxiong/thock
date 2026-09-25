package race

import "time"

// Point is one sample of a racer's position over a round, taken on the
// watcher's own clock.
type Point struct {
	At   time.Duration
	Done float64
	WPM  float64
}

// Reached is when a trace first got as far as done, which is how a time gap is
// measured: if the leader was here 0.8s ago, you are 0.8s behind. That reads
// the same at any speed, where a gap in characters would not.
func Reached(trace []Point, done float64) (time.Duration, bool) {
	for _, p := range trace {
		if p.Done >= done {
			return p.At, true
		}
	}
	return 0, false
}

// Trail spreads a trace across a lane width columns wide: each column holds the
// racer's speed when they passed that point of the text. Gaps between samples
// take the speed before them, so a fast racer whose samples skip columns still
// draws a solid line. Columns before the first measured speed take that speed:
// it is an average over everything typed up to it, so it is the pace those
// columns were covered at.
func Trail(trace []Point, width int) []float64 {
	if width <= 0 {
		return nil
	}
	out := make([]float64, width)
	last, lastCol := 0.0, -1
	for _, p := range trace {
		col := int(p.Done * float64(width-1))
		col = max(0, min(col, width-1))
		for c := lastCol + 1; c < col; c++ {
			out[c] = last
		}
		if col > lastCol {
			out[col] = p.WPM
			lastCol = col
		}
		last = p.WPM
	}
	out = out[:lastCol+1]
	for i, v := range out {
		if v > 0 {
			for j := range i {
				out[j] = v
			}
			break
		}
	}
	return out
}
