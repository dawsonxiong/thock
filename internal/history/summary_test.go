package history

import (
	"math"
	"testing"
	"time"
)

func rec(mode string, dur, words int, wpm, acc float64) Record {
	return Record{
		At:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		Mode: mode, Duration: dur, Words: words,
		WPM: wpm, Accuracy: acc, Consistency: 80,
	}
}

// Setups that are not comparable must never be averaged together: a 30 second
// run and a 120 second run are different events, and pooling them would invent
// a best that was never typed.
func TestSummariseKeepsIncomparableSetupsApart(t *testing.T) {
	rs := []Record{
		rec("time", 30, 0, 70, 96),
		rec("time", 30, 0, 90, 94),
		rec("time", 120, 0, 60, 98),
		rec("words", 0, 25, 80, 92),
		rec("quotes", 0, 0, 50, 99),
		rec("time", 30, 0, 80, 95),
	}

	want := map[string]struct {
		runs int
		best float64
		mean float64
	}{
		"30s":   {3, 90, 80},
		"120s":  {1, 60, 60},
		"25w":   {1, 80, 80},
		"quote": {1, 50, 50},
	}

	got := Summarise(rs)
	if len(got) != len(want) {
		t.Fatalf("got %d groups, want %d: %+v", len(got), len(want), got)
	}
	// Most practised first, so the setup being worked on leads the table.
	if got[0].Key != "30s" {
		t.Errorf("groups lead with %q, want the most practised (30s)", got[0].Key)
	}
	for _, g := range got {
		w, ok := want[g.Key]
		if !ok {
			t.Errorf("unexpected group %q", g.Key)
			continue
		}
		if g.Runs != w.runs || g.Best != w.best || g.WPM != w.mean {
			t.Errorf("%s: runs=%d best=%.0f mean=%.0f, want %d/%.0f/%.0f",
				g.Key, g.Runs, g.Best, g.WPM, w.runs, w.best, w.mean)
		}
	}
}

// The smoothed line has to start where the data starts. Dropping the first
// window-1 runs would leave the line and the bars above it disagreeing about
// where the history begins.
func TestMovingAvgTrailsWithoutDroppingTheStart(t *testing.T) {
	got := MovingAvg([]float64{10, 20, 30, 40, 50}, 3)
	want := []float64{10, 15, 20, 30, 40}

	if len(got) != len(want) {
		t.Fatalf("got %d values, want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("index %d: got %.4f, want %.4f", i, got[i], want[i])
		}
	}
}
