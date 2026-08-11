package chart

import "testing"

// The frame is what decides whether a chart reads as variation or as a solid
// block, and both bounds end up printed on the axis, so they have to be right.
func TestFrameBounds(t *testing.T) {
	tests := []struct {
		name       string
		vals       []float64
		span       float64
		floor, top float64
	}{
		{"floors to a round step below the trough", []float64{62, 75, 98}, 20, 60, 98},
		{"widens a narrow spread to the minimum span", []float64{50, 52, 55}, 20, 35, 55},
		{"never floors below zero", []float64{5, 10}, 20, 0, 10},
		{"a trough at zero stays at zero", []float64{0, 30}, 20, 0, 30},
		{"no data still gives a drawable frame", nil, 20, 0, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			floor, top := Frame(tt.vals, tt.span)
			if floor != tt.floor || top != tt.top {
				t.Errorf("got floor=%.0f top=%.0f, want %.0f/%.0f",
					floor, top, tt.floor, tt.top)
			}
			// The minimum span is honoured unless honouring it would push the
			// floor below zero, where staying at zero wins.
			if floor != 0 && top-floor < tt.span {
				t.Errorf("span %.0f is narrower than the %.0f minimum", top-floor, tt.span)
			}
		})
	}
}

// Widen must hand back a whole number of columns per value, or the bars come
// out different sizes and the chart reads as detail that is not in the data.
func TestWidenGivesEveryValueEqualColumns(t *testing.T) {
	vals, cols := Widen([]float64{1, 2, 3}, 20)
	if cols != 18 || len(vals) != 18 {
		t.Fatalf("got %d columns and %d values, want 18 of each", cols, len(vals))
	}
	for i, v := range vals {
		if want := float64(i/6 + 1); v != want {
			t.Errorf("column %d holds %.0f, want %.0f", i, v, want)
		}
	}
	// A series longer than the plot is left for the resampler instead.
	if _, cols := Widen(make([]float64, 40), 20); cols != 20 {
		t.Errorf("dense series got %d columns, want the full 20", cols)
	}
}
