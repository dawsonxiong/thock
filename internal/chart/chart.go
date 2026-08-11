// Package chart draws block-character plots. It returns plain rows of text and
// takes no view on colour, so callers style the result however they like.
package chart

import "math"

var blocks = [...]rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline compresses values into a single row, resampling to fit width.
func Sparkline(vals []float64, width int) string {
	if len(vals) == 0 || width <= 0 {
		return ""
	}
	v := resample(vals, width)
	lo, hi := bounds(v)
	out := make([]rune, len(v))
	for i, x := range v {
		// Offset into 1..8 so the smallest value still draws a mark; a blank
		// cell would read as missing data rather than as a low value.
		out[i] = blocks[level(x, lo, hi, 7)+1]
	}
	return string(out)
}

// Bars renders a column chart height rows tall, top row first. Values are
// resampled to width columns and scaled against max, or against the data's own
// peak when max is zero.
func Bars(vals []float64, width, height int, max float64) []string {
	if len(vals) == 0 || width <= 0 || height <= 0 {
		return nil
	}
	v := resample(vals, width)
	if max <= 0 {
		_, max = bounds(v)
	}
	if max <= 0 {
		max = 1
	}

	rows := make([]string, height)
	for r := range rows {
		// r counts from the top, so the band below it is (height-1-r).
		band := height - 1 - r
		line := make([]rune, len(v))
		for i, x := range v {
			eighths := int(math.Round(x / max * float64(height*8)))
			rel := eighths - band*8
			switch {
			case rel <= 0:
				line[i] = ' '
			case rel >= 8:
				line[i] = '█'
			default:
				line[i] = blocks[rel]
			}
		}
		rows[r] = string(line)
	}
	return rows
}

// Frame picks the vertical bounds for a bar chart. Anchoring at zero turns a
// steady series into a solid block, so the floor drops to a round multiple of
// minSpan just below the smallest value instead; both bounds are meant to be
// labelled, so the zoom is stated rather than implied. The ceiling is the peak
// itself, so the largest value reaches the top row — rounding it up would leave
// the first row permanently blank. minSpan also stops a flat series from being
// magnified into noise.
func Frame(vals []float64, minSpan float64) (floor, top float64) {
	if len(vals) == 0 {
		return 0, minSpan
	}
	trough, peak := bounds(vals)
	top = peak
	if trough > 0 && minSpan > 0 {
		floor = math.Floor(trough/minSpan) * minSpan
	}
	if top-floor < minSpan {
		floor = math.Max(0, top-minSpan)
	}
	return floor, top
}

// Widen gives each value an equal block of columns when there is room for more
// than one apiece, so a short series draws wide bars rather than a sliver at the
// left edge. Whole-number widths keep every bar the same size, which is what
// stops the result reading as invented detail. It returns the widened values
// and the width they occupy, which can be less than width.
func Widen(vals []float64, width int) ([]float64, int) {
	if len(vals) == 0 || width <= 0 || len(vals) > width {
		return vals, width
	}
	per := width / len(vals)
	out := make([]float64, 0, per*len(vals))
	for _, v := range vals {
		for i := 0; i < per; i++ {
			out = append(out, v)
		}
	}
	return out, per * len(vals)
}

// Marks builds a row of markers under a chart, one column per resampled slot,
// flagging any slot whose source values contained an error.
func Marks(errs []int, width int, mark rune) string {
	if len(errs) == 0 || width <= 0 {
		return ""
	}
	out := make([]rune, width)
	for i := range out {
		out[i] = ' '
	}
	n := len(errs)
	for i, e := range errs {
		if e == 0 {
			continue
		}
		// Centre the mark under its bar. When the data is denser than the plot
		// the bar width rounds to zero and this falls back to the left edge.
		col := i*width/n + width/n/2
		if col >= width {
			col = width - 1
		}
		out[col] = mark
	}
	return string(out)
}

// resample squeezes or stretches values onto exactly n slots, averaging the
// source values that fall into each slot.
func resample(vals []float64, n int) []float64 {
	if len(vals) == n {
		return vals
	}
	out := make([]float64, n)
	for i := range out {
		start := i * len(vals) / n
		end := (i + 1) * len(vals) / n
		if end <= start {
			end = start + 1
		}
		if end > len(vals) {
			end = len(vals)
		}
		sum := 0.0
		for _, v := range vals[start:end] {
			sum += v
		}
		out[i] = sum / float64(end-start)
	}
	return out
}

func bounds(v []float64) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, x := range v {
		lo = math.Min(lo, x)
		hi = math.Max(hi, x)
	}
	if math.IsInf(lo, 1) {
		return 0, 0
	}
	return lo, hi
}

// level maps a value onto 0..steps for block selection.
func level(x, lo, hi float64, steps int) int {
	if hi <= lo {
		if x <= 0 {
			return 0
		}
		return steps / 2
	}
	n := int(math.Round((x - lo) / (hi - lo) * float64(steps)))
	if n < 0 {
		n = 0
	}
	if n > steps {
		n = steps
	}
	return n
}
