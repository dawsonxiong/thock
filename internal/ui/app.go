// Package ui implements the terminal interface: one root model holding a
// screen and an optional overlay. The overlay is kept separate from the screen
// because the options pane sits on top of a live test rather than replacing it.
package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/content"
	"github.com/dawsonxiong/thock/internal/history"
	"github.com/dawsonxiong/thock/internal/layout"
	"github.com/dawsonxiong/thock/internal/render"
	"github.com/dawsonxiong/thock/internal/stats"
	"github.com/dawsonxiong/thock/internal/theme"
	"github.com/dawsonxiong/thock/internal/typing"
)

type screen uint8

const (
	screenTest screen = iota
	screenResults
)

type overlay uint8

const (
	overlayNone overlay = iota
	overlayOptions
)

// Test modes. A time test runs until the clock expires; a words test until a
// fixed number of words is typed; a quotes test until the passage is finished.
const (
	ModeTime   = "time"
	ModeWords  = "words"
	ModeQuotes = "quotes"
)

// Options is the live test configuration, seeded from flags and config.
type Options struct {
	Mode     string
	Duration int
	Words    int
	List     content.List
	Length   content.Length
	Theme    string
	Colour   bool
}

// Model is the whole application state.
type Model struct {
	opts Options
	cfg  config.Config

	eng   *typing.Engine
	quote content.Quote

	running bool
	start   time.Time
	elapsed time.Duration

	screen  screen
	overlay overlay

	tbl   theme.Table
	cache render.Cache
	lines []layout.Line

	live    stats.Result
	res     stats.Result
	isPB    bool
	records []history.Record

	optRow  int
	wrapVer uint64

	caretX, caretY int
	caretOK        bool

	w, h     int
	err      error
	quitting bool
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// New builds the model and loads the first test.
func New(opts Options, cfg config.Config) *Model {
	m := &Model{opts: opts, cfg: cfg, w: 80, h: 24}
	m.applyTheme()
	m.records, _ = history.Load()
	m.reset(true)
	return m
}

func (m *Model) applyTheme() {
	t, ok := theme.Get(m.opts.Theme)
	if !ok {
		t, _ = theme.Get(theme.Default)
	}
	m.tbl = theme.Resolve(t, m.opts.Colour)
	m.cache.Invalidate()
}

// reset starts a fresh test. New text is drawn only when fresh is true, so
// "restart" repeats the same words and "next" draws different ones.
func (m *Model) reset(fresh bool) {
	if fresh || m.eng == nil {
		if m.opts.Mode == ModeQuotes {
			q, err := content.RandomQuote(m.opts.Length)
			if err != nil {
				m.err = err
				return
			}
			m.quote = q
			m.eng = typing.New(content.QuoteWords(q))
		} else {
			// A timed test needs a buffer it cannot exhaust; a words test needs
			// exactly the count asked for, since that is its finish line.
			n := m.opts.Words
			if m.opts.Mode == ModeTime {
				n = content.WordCountFor(m.opts.Duration)
			}
			ws, err := content.Words(m.opts.List, n)
			if err != nil {
				m.err = err
				return
			}
			m.eng = typing.New(ws)
		}
	} else {
		words := make([]string, len(m.eng.Words))
		for i, w := range m.eng.Words {
			words[i] = string(w.Target)
		}
		m.eng = typing.New(words)
	}
	m.running = false
	m.elapsed = 0
	m.screen = screenTest
	m.live = stats.Result{}
	m.res = stats.Result{}
	m.isPB = false
	m.lines = nil
	m.cache.Invalidate()
}

func (m *Model) Init() tea.Cmd { return nil }

// limit is the wall time a test runs for, or zero when it ends on content
// rather than on the clock.
func (m *Model) limit() time.Duration {
	if m.opts.Mode != ModeTime {
		return 0
	}
	return time.Duration(m.opts.Duration) * time.Second
}

func (m *Model) finish() {
	m.running = false
	if lim := m.limit(); lim > 0 && m.elapsed > lim {
		m.elapsed = lim
	}
	m.res = stats.Compute(m.eng, m.elapsed)
	m.screen = screenResults

	// A run this short cannot produce a real score, so it is shown but never
	// recorded — otherwise it would sit at the top of the bests permanently.
	if stats.TooShort(m.elapsed) {
		return
	}

	rec := history.Record{
		At:          time.Now(),
		Mode:        m.opts.Mode,
		Duration:    int(m.elapsed.Round(time.Second).Seconds()),
		WPM:         m.res.WPM,
		Raw:         m.res.Raw,
		Accuracy:    m.res.Accuracy,
		Consistency: m.res.Consistency,
		Chars:       [4]int{m.res.CorrectChars, m.res.IncorrectChars, m.res.ExtraChars, m.res.MissedChars},
	}
	switch m.opts.Mode {
	case ModeTime:
		rec.Duration = m.opts.Duration
		rec.List = string(m.opts.List)
	case ModeWords:
		rec.Words = m.opts.Words
		rec.List = string(m.opts.List)
	default:
		rec.Source = m.quote.Attribution()
	}
	m.isPB = history.IsPB(m.records, rec)
	if err := history.Append(rec); err == nil {
		m.records = append(m.records, rec)
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width != m.w {
			m.lines = nil
		}
		m.w, m.h = msg.Width, msg.Height
		m.cache.Invalidate()
		return m, nil

	case tickMsg:
		if !m.running {
			return m, nil
		}
		m.elapsed = time.Since(m.start)
		if lim := m.limit(); lim > 0 && m.elapsed >= lim {
			m.finish()
			return m, nil
		}
		m.live = stats.Compute(m.eng, m.elapsed)
		return m, tick()

	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	if m.screen == screenTest && m.overlay == overlayNone && m.err == nil {
		if x, y, ok := m.caret(); ok {
			c := tea.NewCursor(x, y)
			c.Blink = !m.running
			v.Cursor = c
		}
	}
	return v
}
