package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/history"
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

	// The character tallies spell themselves out only when the box has room.
	narrow := mk(base)
	narrow.Update(tea.WindowSizeMsg{Width: 58, Height: 24})
	narrow.res = m.res
	narrow.screen = screenResults
	show("results — 58x24, tallies fall back to the compact form", narrow)

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

	for _, size := range [][2]int{{96, 30}, {80, 24}, {64, 18}} {
		s := mk(base)
		s.records = fakeHistory()
		s.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		s.openStats()
		show(fmt.Sprintf("stats — all, %dx%d", size[0], size[1]), s)
		if size[0] == 96 {
			key(s, tea.KeyRight)
			show("stats — filtered to one setup", s)
		}
	}

	empty := mk(base)
	empty.records = nil
	empty.openStats()
	show("stats — no history yet", empty)
}

// fakeHistory is a plausible few months of practice: mostly 30 second runs
// that improve over time, with a scatter of other setups around them.
func fakeHistory() []history.Record {
	day := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	var out []history.Record
	add := func(mode string, dur, words int, wpm, acc, cons float64) {
		day = day.Add(19 * time.Hour)
		out = append(out, history.Record{
			At: day, Mode: mode, Duration: dur, Words: words, List: "1k",
			WPM: wpm, Raw: wpm * 1.07, Accuracy: acc, Consistency: cons,
		})
	}
	for i := 0; i < 46; i++ {
		drift := float64(i) * 0.55
		wobble := float64((i*37)%13) - 6
		add("time", 30, 0, 68+drift+wobble, 93+float64((i*7)%6), 74+float64((i*11)%17))
		switch i % 7 {
		case 3:
			add("time", 60, 0, 64+drift*0.8+wobble, 94+float64((i*5)%5), 77+float64((i*3)%14))
		case 5:
			add("words", 0, 25, 71+drift+wobble, 95+float64((i*3)%4), 80+float64((i*13)%12))
		case 6:
			add("quotes", 0, 0, 66+drift*0.6+wobble, 96+float64(i%3), 82+float64((i*17)%10))
		}
	}
	return out
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
