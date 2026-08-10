package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/layout"
	"github.com/dawsonxiong/thock/internal/stats"
)

// isolate points config and history at a scratch directory so running the
// tests never touches the real ~/.config or ~/.local/share.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func newTestModel(t *testing.T, w, h int) *Model {
	t.Helper()
	isolate(t)
	m := New(Options{
		Mode: ModeTime, Duration: 30, Words: 25, List: "1k", Length: "any",
		Theme: "mono", Colour: false,
	}, config.Default())
	if m.err != nil {
		t.Fatalf("setup: %v", m.err)
	}
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func press(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func key(m *Model, code rune) { m.Update(tea.KeyPressMsg{Code: code}) }

// strip removes escape sequences so assertions read against visible text.
func strip(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// Typing the target text exactly must leave the visible text unchanged: the
// characters are recoloured, never rewritten.
func TestTypingPreservesVisibleText(t *testing.T) {
	m := newTestModel(t, 80, 24)
	want := string(m.eng.Words[0].Target)

	press(m, want)
	got := strip(m.View().Content)
	if !strings.Contains(got, want) {
		t.Fatalf("typed word %q missing from frame:\n%s", want, got)
	}
	if m.eng.Words[0].Correct() != true {
		t.Errorf("word should be correct after typing it exactly")
	}
}

// Overtyping a word appends the extra characters to what is displayed, which is
// what pushes the line and is why the layout has to re-wrap.
func TestExtraCharsAppearAndBumpLayoutVersion(t *testing.T) {
	m := newTestModel(t, 80, 24)
	before := m.eng.Version()

	press(m, string(m.eng.Words[0].Target)+"zz")

	if m.eng.Version() == before {
		t.Error("version unchanged; layout would not re-wrap after overtyping")
	}
	got := strip(m.View().Content)
	if !strings.Contains(got, string(m.eng.Words[0].Target)+"zz") {
		t.Errorf("extra characters not rendered:\n%s", got)
	}
}

// The caret must sit exactly after the typed prefix, measured in terminal
// cells. A drifting caret is the most visible possible bug.
func TestCaretTracksTypedPrefix(t *testing.T) {
	m := newTestModel(t, 80, 24)
	w0 := string(m.eng.Words[0].Target)

	m.View()
	x0, _, ok := m.caret()
	if !ok {
		t.Fatal("caret not reported on a fresh test")
	}
	press(m, w0)
	m.View()
	x1, _, _ := m.caret()

	if x1-x0 != len([]rune(w0)) {
		t.Errorf("caret advanced %d cells, want %d for %q", x1-x0, len([]rune(w0)), w0)
	}

	// A space moves it one further, past the separator.
	press(m, " ")
	m.View()
	x2, _, _ := m.caret()
	if x2-x1 != 1 {
		t.Errorf("caret advanced %d cells over the space, want 1", x2-x1)
	}
}

// Resizing must rewrap without disturbing what the user typed.
func TestResizePreservesTypedState(t *testing.T) {
	m := newTestModel(t, 80, 24)
	press(m, string(m.eng.Words[0].Target)+" ")
	press(m, "abc")

	typed := string(m.eng.Current().Typed)
	idx := m.eng.WordIdx

	m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m.View()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.View()

	if got := string(m.eng.Current().Typed); got != typed {
		t.Errorf("typed text changed across resize: %q -> %q", typed, got)
	}
	if m.eng.WordIdx != idx {
		t.Errorf("word index changed across resize: %d -> %d", idx, m.eng.WordIdx)
	}
	if _, _, ok := m.caret(); !ok {
		t.Error("caret lost after resize")
	}
}

// The options overlay must leave the test running underneath it, and the theme
// picker must take effect immediately.
func TestOptionsOverlayKeepsTestVisible(t *testing.T) {
	m := newTestModel(t, 80, 30)
	w0 := string(m.eng.Words[0].Target)
	press(m, w0)

	key(m, tea.KeyTab)
	frame := strip(m.View().Content)

	if !strings.Contains(frame, "mode") || !strings.Contains(frame, "theme") {
		t.Errorf("options pane not shown:\n%s", frame)
	}
	if !strings.Contains(frame, w0) {
		t.Errorf("test text hidden behind the overlay:\n%s", frame)
	}

	// Move to the theme row and change it; the frame must survive the switch.
	for range 5 {
		m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.opts.Theme == "mono" {
		t.Error("theme did not change when cycling the picker")
	}
	if strip(m.View().Content) == "" {
		t.Error("frame empty after theme change")
	}
}

// A words test ends on its own word count, and momentum keystrokes landing on
// the results screen must not skip past it.
func TestWordsModeEndsOnCountAndIgnoresLetters(t *testing.T) {
	isolate(t)
	m := New(Options{
		Mode: ModeWords, Duration: 30, Words: 10, List: "1k", Theme: "mono",
	}, config.Default())
	if m.err != nil {
		t.Fatalf("setup: %v", m.err)
	}
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})

	if len(m.eng.Words) != 10 {
		t.Fatalf("engine has %d words, want exactly 10", len(m.eng.Words))
	}

	for i := range m.eng.Words {
		press(m, string(m.eng.Words[i].Target))
		if i < len(m.eng.Words)-1 {
			press(m, " ")
		}
	}
	if m.screen != screenResults {
		t.Fatal("typing all 10 words did not finish the test")
	}

	// The keystrokes that used to advance the screen: r, n, q and space.
	before := m.eng
	press(m, "rnq ")
	if m.screen != screenResults {
		t.Error("a letter left the results screen")
	}
	if m.eng != before {
		t.Error("a letter started a new test from the results screen")
	}
}

// A short words test still has a pacing chart. Per-second sampling gives a
// five second run only five data points, which must widen into bars rather
// than suppress the chart.
func TestShortRunStillDrawsChart(t *testing.T) {
	m := newTestModel(t, 96, 32)
	for _, secs := range []int{3, 5, 9, 15, 60} {
		m.res.Samples = make([]stats.Sample, secs)
		for i := range m.res.Samples {
			m.res.Samples[i] = stats.Sample{Second: i + 1, Raw: 100 + float64(i%3)*20}
		}
		rows := m.chartRows(m.boxWidth())
		if len(rows) < chartHeight {
			t.Errorf("%ds run produced %d chart rows, want at least %d",
				secs, len(rows), chartHeight)
			continue
		}
		// Every row of the plot must be the same width or the axis shears.
		want := layout.Width(strip(rows[0]))
		for i, r := range rows {
			if got := layout.Width(strip(r)); got != want {
				t.Errorf("%ds run: chart row %d is %d cells, row 0 is %d",
					secs, i, got, want)
			}
		}
	}
}

// Tab then enter is the restart gesture, from a running test or from results.
func TestTabEnterRestarts(t *testing.T) {
	m := newTestModel(t, 80, 30)
	press(m, string(m.eng.Words[0].Target)+" ")
	if m.eng.WordIdx == 0 {
		t.Fatal("setup: expected to be past the first word")
	}

	key(m, tea.KeyTab)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.overlay != overlayNone {
		t.Error("options pane still open after enter")
	}
	if m.eng.WordIdx != 0 || m.eng.Current().Started() {
		t.Errorf("test not restarted: idx=%d typed=%q",
			m.eng.WordIdx, string(m.eng.Current().Typed))
	}

	// The same gesture has to work from the results screen, not just mid-test.
	m.screen = screenResults
	key(m, tea.KeyTab)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != screenTest {
		t.Error("tab+enter from results did not start a new test")
	}
}

// Finishing a quote runs the whole pipeline: engine to stats to results screen.
func TestQuoteRunReachesResults(t *testing.T) {
	isolate(t)
	m := New(Options{
		Mode: ModeQuotes, Duration: 30, Words: 25, List: "1k", Length: "short",
		Theme: "mono", Colour: false,
	}, config.Default())
	if m.err != nil {
		t.Fatalf("setup: %v", m.err)
	}
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})

	for i := range m.eng.Words {
		press(m, string(m.eng.Words[i].Target))
		if i < len(m.eng.Words)-1 {
			press(m, " ")
		}
	}

	if m.screen != screenResults {
		t.Fatalf("did not reach results after typing the whole quote")
	}
	if m.res.Accuracy < 99.9 {
		t.Errorf("accuracy = %v, want 100 for a clean run", m.res.Accuracy)
	}
	frame := strip(m.View().Content)
	for _, want := range []string{"wpm", "acc", "raw", "—"} {
		if !strings.Contains(frame, want) {
			t.Errorf("results frame missing %q:\n%s", want, frame)
		}
	}
}
