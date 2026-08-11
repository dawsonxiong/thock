package history

import (
	"math"
	"sort"
	"time"
)

// Group is every record sharing a comparability key, reduced to the figures
// worth showing side by side.
type Group struct {
	Key         string
	Runs        int
	Best        float64
	WPM         float64 // mean
	Accuracy    float64 // mean
	Consistency float64 // mean
	Last        time.Time
}

// Summarise reduces records to one Group per comparable key, ordered by run
// count so the setups actually practised come first. Groups are never merged:
// a 30 second run is not comparable with a 120 second one.
func Summarise(rs []Record) []Group {
	idx := make(map[string]int)
	var out []Group
	for _, r := range rs {
		k := r.Key()
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, Group{Key: k})
		}
		g := &out[i]
		g.Runs++
		g.Best = math.Max(g.Best, r.WPM)
		if r.At.After(g.Last) {
			g.Last = r.At
		}
		// Sums for now; divided into means below.
		g.WPM += r.WPM
		g.Accuracy += r.Accuracy
		g.Consistency += r.Consistency
	}
	for i := range out {
		n := float64(out[i].Runs)
		out[i].WPM /= n
		out[i].Accuracy /= n
		out[i].Consistency /= n
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Runs > out[j].Runs })
	return out
}

// Series is the per-run tracks for a set of records, oldest first.
type Series struct {
	WPM         []float64
	Accuracy    []float64
	Consistency []float64
}

// Track extracts the tracks for one comparable key, or for every record when
// key is empty. The log is append-only, so records arrive in chronological
// order and the tracks inherit it.
func Track(rs []Record, key string) Series {
	var s Series
	for _, r := range rs {
		if key != "" && r.Key() != key {
			continue
		}
		s.WPM = append(s.WPM, r.WPM)
		s.Accuracy = append(s.Accuracy, r.Accuracy)
		s.Consistency = append(s.Consistency, r.Consistency)
	}
	return s
}

// Summary reduces a series to the figures shown above a chart.
func (s Series) Summary() (best, wpm, acc, cons float64) {
	for _, v := range s.WPM {
		best = math.Max(best, v)
	}
	return best, mean(s.WPM), mean(s.Accuracy), mean(s.Consistency)
}

// MovingAvg is the trailing mean of v over at most window values, one entry per
// input value. Early entries average the little history there is rather than
// being dropped, so the line starts where the data starts.
func MovingAvg(v []float64, window int) []float64 {
	if len(v) == 0 || window < 1 {
		return nil
	}
	out := make([]float64, len(v))
	sum := 0.0
	for i, x := range v {
		sum += x
		if i >= window {
			sum -= v[i-window]
		}
		n := i + 1
		if n > window {
			n = window
		}
		out[i] = sum / float64(n)
	}
	return out
}

// Trend compares the mean of the first half of a series with the second half,
// which is the smallest honest answer to "am I getting faster".
func Trend(v []float64) (early, late float64) {
	if len(v) < 2 {
		return 0, 0
	}
	mid := len(v) / 2
	return mean(v[:mid]), mean(v[mid:])
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}
