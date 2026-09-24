package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
)

// BenchmarkKeystroke measures the full per-keystroke cost the user waits on:
// one Update for the key press followed by one View to build the next frame.
// Run: go test ./internal/ui -bench Keystroke -benchmem
func BenchmarkKeystroke(b *testing.B) {
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	b.Setenv("XDG_DATA_HOME", b.TempDir())
	m := newBenchModel(b, 120, 40)
	target := benchTarget(m, 4000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%len(target) == 0 && i > 0 {
			// start a fresh run so every keystroke lands on live text, not the results screen
			b.StopTimer()
			m = newBenchModel(b, 120, 40)
			b.StartTimer()
		}
		r := target[i%len(target)]
		if r == ' ' {
			m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		} else {
			m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		_ = m.View()
	}
}

// BenchmarkView isolates frame construction, which Bubble Tea runs at up to
// 120 FPS independent of input.
func BenchmarkView(b *testing.B) {
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	b.Setenv("XDG_DATA_HOME", b.TempDir())
	m := newBenchModel(b, 120, 40)
	target := benchTarget(m, 60)
	for _, r := range target {
		if r == ' ' {
			m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		} else {
			m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func newBenchModel(b *testing.B, w, h int) *Model {
	b.Helper()
	m := New(Options{
		Mode: ModeWords, Words: 1000, List: "1k", Length: "any",
		Theme: "mono", Colour: false,
	}, config.Default())
	if m.err != nil {
		b.Fatalf("setup: %v", m.err)
	}
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// benchTarget returns up to n runes of the text the model expects, so every
// keystroke is a correct one and the run never ends early.
func benchTarget(m *Model, n int) []rune {
	var out []rune
	for _, w := range m.eng.Words {
		out = append(out, w.Target...)
		out = append(out, ' ')
		if len(out) >= n {
			break
		}
	}
	return out
}
