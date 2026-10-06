package theme

import (
	"strings"
	"testing"
)

// A lane's tones brighten toward the racer's own colour, which the fastest
// tone is exactly, so the lane still matches the racer's name and caret.
func TestLaneShadesRampToTheRacerColour(t *testing.T) {
	th, _ := Get("tokyonight")
	tb := Resolve(th, true)
	for i := range th.Racers {
		ramp := tb.RacerLane[i]
		if ramp[LaneShades-1] != tb.Racer[i] {
			t.Errorf("racer %d: fastest tone %q is not the racer colour %q", i, ramp[LaneShades-1], tb.Racer[i])
		}
		for j := 1; j < LaneShades; j++ {
			if ramp[j] == ramp[j-1] {
				t.Errorf("racer %d: tones %d and %d are the same", i, j-1, j)
			}
		}
	}
	if tb.AccentLane[LaneShades-1] != tb.Accent {
		t.Errorf("your fastest tone %q is not the accent %q", tb.AccentLane[LaneShades-1], tb.Accent)
	}
}

// Without hex colours to blend, the slower half of the ramp is faint.
func TestLaneShadesFallBackToFaint(t *testing.T) {
	for _, tc := range []struct {
		name   string
		colour bool
	}{{"terminal", true}, {"tokyonight", false}} {
		th, _ := Get(tc.name)
		ramp := Resolve(th, tc.colour).RacerLane[0]
		for j, s := range ramp {
			faint := strings.Contains(s, "[2m") || strings.Contains(s, "[2;") || strings.Contains(s, ";2m")
			if want := j < LaneShades/2; faint != want {
				t.Errorf("%s colour=%v: tone %d %q faint=%v, want %v", tc.name, tc.colour, j, s, faint, want)
			}
		}
	}
}
