package ui

import (
	"strconv"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/content"
	"github.com/dawsonxiong/thock/internal/theme"
)

func (m *Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		m.Close()
		return m, tea.Quit
	}
	if m.err != nil {
		m.quitting = true
		return m, tea.Quit
	}
	if m.overlay == overlayOptions {
		return m.onOptionsKey(msg)
	}
	switch m.screen {
	case screenStats:
		return m.onStatsKey(msg)
	case screenResults:
		return m.onResultsKey(msg)
	case screenBrowse:
		return m.onBrowseKey(msg)
	case screenLobby, screenPodium:
		return m.onLobbyKey(msg)
	case screenRace:
		return m.onRaceKey(msg)
	}
	return m.onTestKey(msg)
}

// openStats remembers the current screen so that leaving the stats view returns
// to it, rather than discarding a finished result or a loaded test.
func (m *Model) openStats() {
	m.returnTo = m.screen
	m.screen = screenStats
	m.statsFilter = 0
}

func (m *Model) onStatsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+s":
		m.screen = m.returnTo
	case "left", "h":
		m.cycleFilter(-1)
	case "right", "l":
		m.cycleFilter(+1)
	}
	return m, nil
}

func (m *Model) onTestKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "esc":
		m.reset(false)
		return m, nil
	case "tab":
		m.overlay = overlayOptions
		m.optRow = 0
		return m, nil
	case "ctrl+s":
		// Ignored once the clock is running: there is no way back into a test
		// mid-flow, so opening stats would silently cost the run.
		if !m.running {
			m.openStats()
		}
		return m, nil
	case "ctrl+r":
		// Like stats, racing is only offered between tests.
		if !m.running {
			return m, m.openBrowse()
		}
		return m, nil
	// Terminals without key disambiguation report ctrl+backspace as one of
	// these, so all three delete a word.
	case "ctrl+backspace", "ctrl+h", "ctrl+w", "alt+backspace":
		m.eng.DeleteWord(m.since())
		return m, nil
	case "backspace":
		m.eng.Backspace(m.since())
		return m, nil
	}

	key := msg.Key()
	if key.Text == "" {
		return m, nil
	}
	r := []rune(key.Text)
	if len(r) != 1 || unicode.IsControl(r[0]) {
		return m, nil
	}

	var cmd tea.Cmd
	if !m.running {
		m.running = true
		m.start = time.Now()
		cmd = tick()
	}
	at := m.since()
	if r[0] == ' ' {
		m.eng.Space(at)
	} else {
		m.eng.TypeRune(r[0], at)
	}
	if m.testComplete() {
		m.elapsed = m.since()
		m.finish()
	}
	return m, cmd
}

// testComplete reports whether the content has run out. Timed tests never end
// this way; they end on the clock. A quote ends as soon as its final word is
// filled in, without needing a trailing space that is not in the text.
func (m *Model) testComplete() bool {
	if m.limit() > 0 {
		return false
	}
	if m.eng.Done() {
		return true
	}
	if m.eng.WordIdx == len(m.eng.Words)-1 {
		w := m.eng.Current()
		return w != nil && len(w.Typed) >= len(w.Target)
	}
	return false
}

func (m *Model) since() time.Duration {
	if !m.running {
		return 0
	}
	return time.Since(m.start)
}

// onResultsKey deliberately ignores printable characters. Finishing a test
// leaves you mid-flow, and any letter that advanced the screen would fire on
// the momentum keystrokes that land right after the last word.
func (m *Model) onResultsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.reset(true) // fresh text
	case "esc":
		m.reset(false) // same text again
	case "tab":
		m.overlay = overlayOptions
		m.optRow = 0
	case "ctrl+s":
		m.openStats()
	case "ctrl+r":
		return m, m.openBrowse()
	}
	return m, nil
}

// The selectable presets, shared by the options pane and the config bar so the
// two can never disagree about what is on offer.
var (
	durations  = []string{"15", "30", "60", "120"}
	wordCounts = []string{"10", "25", "50", "100"}
)

// optRow is one line of the options pane.
type optRow struct {
	label  string
	values []string
	idx    int
	apply  func(int)
}

// optionRows builds the pane contents for the current mode. Rows that do not
// apply to the active mode are simply absent rather than disabled.
func (m *Model) optionRows() []optRow {
	modes := []string{ModeTime, ModeWords, ModeQuotes}
	rows := []optRow{{
		label:  "mode",
		values: modes,
		idx:    indexOf(modes, m.opts.Mode),
		apply:  func(i int) { m.opts.Mode = modes[i]; m.reset(true) },
	}}

	switch m.opts.Mode {
	case ModeTime:
		rows = append(rows, optRow{
			label:  "time",
			values: durations,
			idx:    indexOf(durations, strconv.Itoa(m.opts.Duration)),
			apply: func(i int) {
				m.opts.Duration, _ = strconv.Atoi(durations[i])
				m.reset(true)
			},
		}, m.listRow())
	case ModeWords:
		rows = append(rows, optRow{
			label:  "words",
			values: wordCounts,
			idx:    indexOf(wordCounts, strconv.Itoa(m.opts.Words)),
			apply: func(i int) {
				m.opts.Words, _ = strconv.Atoi(wordCounts[i])
				m.reset(true)
			},
		}, m.listRow())
	default:
		lens := []string{"any", "short", "medium", "long"}
		rows = append(rows, optRow{
			label:  "length",
			values: lens,
			idx:    indexOf(lens, string(m.opts.Length)),
			apply:  func(i int) { m.opts.Length = content.Length(lens[i]); m.reset(true) },
		})
	}

	names := theme.Names()
	rows = append(rows, optRow{
		label:  "theme",
		values: names,
		idx:    indexOf(names, m.opts.Theme),
		apply:  func(i int) { m.opts.Theme = names[i]; m.applyTheme() },
	})
	return rows
}

// listRow is shared by the time and words modes, which both draw from a list.
func (m *Model) listRow() optRow {
	lists := []string{"1k", "5k"}
	return optRow{
		label:  "list",
		values: lists,
		idx:    indexOf(lists, string(m.opts.List)),
		apply:  func(i int) { m.opts.List = content.List(lists[i]); m.reset(true) },
	}
}

func (m *Model) onOptionsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rows := m.optionRows()
	switch msg.String() {
	// Tab then enter is the restart gesture, from either a running test or the
	// results screen. Esc and tab back out without disturbing the test.
	case "enter":
		m.overlay = overlayNone
		m.saveConfig()
		m.reset(true)
	case "esc", "tab":
		m.overlay = overlayNone
		m.saveConfig()
	case "up", "k":
		if m.optRow > 0 {
			m.optRow--
		}
	case "down", "j":
		if m.optRow < len(rows)-1 {
			m.optRow++
		}
	case "left", "h":
		m.cycle(rows, -1)
	case "right", "l":
		m.cycle(rows, +1)
	}
	return m, nil
}

// cycle moves the selected row's value, wrapping at both ends. The change is
// applied immediately so the theme picker previews as you move through it.
func (m *Model) cycle(rows []optRow, delta int) {
	if m.optRow >= len(rows) {
		return
	}
	r := rows[m.optRow]
	n := len(r.values)
	if n == 0 {
		return
	}
	next := (r.idx + delta + n) % n
	r.apply(next)
}

func (m *Model) saveConfig() {
	m.cfg.Theme = m.opts.Theme
	m.cfg.Test = config.Test{
		Mode:     m.opts.Mode,
		Duration: m.opts.Duration,
		Words:    m.opts.Words,
		List:     string(m.opts.List),
		Length:   string(m.opts.Length),
	}
	_ = config.Save(m.cfg)
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return 0
}
