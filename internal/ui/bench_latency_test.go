package ui

import (
	"sort"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
)

// TestKeystrokeLatencyDistribution times every keystroke of full typing runs
// individually (Update + View) and reports the distribution. It is a
// measurement, not an assertion: run with -run KeystrokeLatency -v.
func TestKeystrokeLatencyDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement only")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var samples []time.Duration
	for run := 0; run < 50; run++ {
		m := New(Options{Mode: ModeWords, Words: 1000, List: "1k", Length: "any", Theme: "mono", Colour: false}, config.Default())
		if m.err != nil {
			t.Fatal(m.err)
		}
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		n := 0
		for _, w := range m.eng.Words {
			for _, r := range append(append([]rune{}, w.Target...), ' ') {
				msg := tea.KeyPressMsg{Code: r, Text: string(r)}
				if r == ' ' {
					msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
				}
				start := time.Now()
				m.Update(msg)
				_ = m.View()
				samples = append(samples, time.Since(start))
				n++
			}
			if n >= 2000 {
				break
			}
		}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p := func(q float64) time.Duration { return samples[int(float64(len(samples)-1)*q)] }
	t.Logf("keystrokes=%d p50=%v p95=%v p99=%v p99.9=%v max=%v", len(samples), p(0.50), p(0.95), p(0.99), p(0.999), samples[len(samples)-1])
}
