package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/stats"
)

// TestDump is a visual harness, not an assertion. Run it with -v to eyeball
// real frames: go test ./internal/ui -run TestDump -v
func TestDump(t *testing.T) {
	isolate(t)
	show := func(label string, m *Model) {
		fmt.Printf("\n\x1b[7m %s \x1b[0m\n%s\n", label, m.View().Content)
	}

	mk := func(o Options) *Model {
		m := New(o, config.Default())
		if m.err != nil {
			t.Fatal(m.err)
		}
		m.Update(tea.WindowSizeMsg{Width: 96, Height: 26})
		return m
	}

	base := Options{Mode: ModeTime, Duration: 30, Words: 25, List: "1k", Length: "any", Theme: "mono", Colour: true}

	m := mk(base)
	show("idle — banner and config bar", m)

	// Type the first few words, fumbling one and overtyping another.
	press(m, string(m.eng.Words[0].Target)+" ")
	press(m, string(m.eng.Words[1].Target[:1])+"zx ")
	press(m, string(m.eng.Words[2].Target)+"qq ")
	press(m, string(m.eng.Words[3].Target[:2]))
	m.running = true
	m.start = time.Now().Add(-8 * time.Second)
	m.elapsed = 8 * time.Second
	m.live = stats.Compute(m.eng, m.elapsed)
	show("typing — correct, wrong, extra, pending", m)

	key(m, tea.KeyTab)
	show("options overlay over a live test", m)
	key(m, tea.KeyTab)

	// Results, with a plausible spread of per-second samples.
	m.elapsed = 30 * time.Second
	m.finish()
	m.res.Samples = fakeSamples()
	m.res.WPM, m.res.Raw, m.res.Accuracy, m.res.Consistency = 104, 112, 97, 88
	m.isPB = true
	show("results — mono", m)

	for _, name := range []string{"gruvbox", "nord", "amber"} {
		m2 := mk(Options{Mode: ModeTime, Duration: 30, Words: 25, List: "1k", Theme: name, Colour: true})
		press(m2, string(m2.eng.Words[0].Target)+" "+string(m2.eng.Words[1].Target[:1])+"zx ")
		show("theme — "+name, m2)
	}

	q := mk(Options{Mode: ModeQuotes, Duration: 30, Words: 25, List: "1k", Length: "short", Theme: "mono", Colour: true})
	show("quotes — idle", q)
	for i := range q.eng.Words {
		press(q, string(q.eng.Words[i].Target))
		if i < len(q.eng.Words)-1 {
			press(q, " ")
		}
	}
	q.res.Samples = fakeSamples()
	show("quotes — results with attribution", q)

	nc := mk(Options{Mode: ModeTime, Duration: 30, Words: 25, List: "1k", Theme: "mono", Colour: false})
	press(nc, string(nc.eng.Words[0].Target)+" "+string(nc.eng.Words[1].Target[:1])+"zx ")
	show("NO_COLOR — state by underline and dim", nc)
}

func fakeSamples() []stats.Sample {
	vals := []float64{40, 62, 78, 91, 104, 98, 112, 106, 118, 99, 121, 108,
		114, 96, 120, 111, 103, 117, 109, 122, 113, 105, 119, 110,
		116, 102, 121, 107, 115, 104}
	out := make([]stats.Sample, len(vals))
	for i, v := range vals {
		e := 0
		if i == 4 || i == 13 || i == 21 {
			e = 1
		}
		out[i] = stats.Sample{Second: i + 1, Raw: v, WPM: v * 0.93, Errors: e}
	}
	return out
}
