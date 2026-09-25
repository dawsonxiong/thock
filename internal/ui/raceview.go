package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/dawsonxiong/thock/internal/chart"
	"github.com/dawsonxiong/thock/internal/layout"
	"github.com/dawsonxiong/thock/internal/race"
	"github.com/dawsonxiong/thock/internal/render"
	"github.com/dawsonxiong/thock/internal/typing"
)

const (
	// maxLaneName is the widest name a lane gives room to.
	maxLaneName = 8
	// laneRight is the width of the figure at the end of each lane.
	laneRight = 8
	// laneFloor is the least speed a lane scales its trail to, so a slow
	// start does not draw at full height.
	laneFloor = 60
)

// racerStyle is a player's colour: the accent for you, and the theme's racer
// colours for everyone else, in the order they joined, so a colour stays with
// a player for as long as the room lasts.
func (m *Model) racerStyle(id int) (fg, ghost string) {
	s := m.race
	if id == s.me {
		return m.tbl.Accent, ghostOf(m.tbl.Accent)
	}
	i := 0
	for _, p := range s.st.Players {
		if p.ID == id {
			break
		}
		if p.ID != s.me {
			i++
		}
	}
	return m.tbl.Racer[i%len(m.tbl.Racer)], m.tbl.Ghost[i%len(m.tbl.Ghost)]
}

// ghostOf turns a foreground style into a caret block of the same colour.
func ghostOf(fg string) string {
	if strings.HasPrefix(fg, "\x1b[") {
		return "\x1b[7;" + fg[2:]
	}
	return "\x1b[7m"
}

// racerName is how a player is labelled: you are always "you".
func (m *Model) racerName(id int, name string) string {
	if id == m.race.me {
		return "you"
	}
	return name
}

// wrapFor is the line layout of an engine at a width, cached for the model's
// own engine.
func (m *Model) wrapFor(e *typing.Engine, box int) []layout.Line {
	if e != m.eng {
		return layout.Wrap(e, box)
	}
	if m.lines == nil || m.wrapVer != m.eng.Version() {
		m.lines = layout.Wrap(m.eng, box)
		m.wrapVer = m.eng.Version()
	}
	return m.lines
}

// ghostMark places another racer's caret in your text. At the end of a word
// it sits on the space after it, or at the start of the next line when the
// word ends a line and there is no space to sit on.
func ghostMark(e *typing.Engine, lines []layout.Line, word, char int, style string) (render.Mark, bool) {
	if word < 0 || word >= len(e.Words) {
		return render.Mark{}, false
	}
	if char < len(e.Words[word].Target) {
		return render.Mark{Word: word, Char: max(char, 0), Style: style}, true
	}
	if l := layout.LineOf(lines, word); l < len(lines) && lines[l].Last == word {
		if word+1 >= len(e.Words) {
			return render.Mark{}, false
		}
		return render.Mark{Word: word + 1, Style: style}, true
	}
	return render.Mark{Word: word, Sep: true, Style: style}, true
}

// lane is one racer's row on the track.
type lane struct {
	id       int
	name     string
	style    string
	trace    []race.Point
	done     float64
	wpm      float64
	finished bool
	out      bool
	time     time.Duration
}

// liveLanes builds the track from the room's latest update, with your own
// lane taken straight from your engine so it never lags your typing.
func (m *Model) liveLanes() []lane {
	s := m.race
	var out []lane
	for _, p := range s.st.Players {
		if !p.Racing || s.round == nil || s.st.Round != s.round.Round {
			continue
		}
		fg, _ := m.racerStyle(p.ID)
		l := lane{id: p.ID, name: m.racerName(p.ID, p.Name), style: fg, trace: s.traces[p.ID],
			done: p.Done, wpm: p.WPM, finished: p.Finished, out: p.Out, time: race.Dur(p.Time)}
		if p.ID == s.me {
			_, _, l.done = race.Position(m.eng)
			l.wpm, l.finished, l.out = m.live.WPM, s.done && !s.gaveUp, s.gaveUp
			l.time = m.elapsed
		}
		out = append(out, l)
	}
	return youFirst(out, s.me)
}

// replayLanes builds the track from the replayed engines.
func (m *Model) replayLanes(now time.Time) []lane {
	s := m.race
	rp := s.replay
	t := rp.clock(now)
	var out []lane
	for _, st := range s.results.Standings {
		e := rp.engines[st.ID]
		if e == nil {
			continue
		}
		fg, _ := m.racerStyle(st.ID)
		l := lane{id: st.ID, name: m.racerName(st.ID, st.Name), style: fg, trace: rp.traces[st.ID]}
		_, _, l.done = race.Position(e)
		if n := len(l.trace); n > 0 {
			l.wpm = l.trace[n-1].WPM
		}
		if race.Complete(e) {
			l.finished, l.time = true, race.Dur(st.Time)
		} else if keys := s.results.Keys[st.ID]; st.Place == 0 && len(keys) > 0 && t >= keys[len(keys)-1].At {
			l.out = true
		}
		out = append(out, l)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].id < out[j].id })
	return youFirst(out, s.me)
}

func youFirst(ls []lane, me int) []lane {
	for i, l := range ls {
		if l.id == me && i > 0 {
			copy(ls[1:i+1], ls[:i])
			ls[0] = l
		}
	}
	return ls
}

// ranks orders lanes the way the standings will: finishers by time, then by
// distance covered.
func ranks(ls []lane) map[int]int {
	order := append([]lane(nil), ls...)
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.finished != b.finished {
			return a.finished
		}
		if a.finished {
			return a.time < b.time
		}
		if a.out != b.out {
			return !a.out
		}
		return a.done > b.done
	})
	out := make(map[int]int, len(order))
	for i, l := range order {
		out[l.id] = i + 1
	}
	return out
}

// laneRows draws the track: one lane per racer, the trail behind each one
// shaded by how fast they were going when they passed that point, so you can
// see where a lead was built as well as who has it.
func (m *Model) laneRows(box int, ls []lane, started bool) []string {
	nameW := 3
	for _, l := range ls {
		nameW = max(nameW, min(layout.Width(l.name), maxLaneName))
	}
	track := box - 1 - nameW - 2 - 2 - laneRight
	if track < 8 {
		return nil
	}

	// One scale for every lane, so a taller trail really is a faster one.
	hi := float64(laneFloor)
	for _, l := range ls {
		hi = math.Max(hi, l.wpm*1.15)
	}
	rank := ranks(ls)

	out := make([]string, 0, len(ls))
	for _, l := range ls {
		pos := int(math.Round(l.done * float64(track-1)))
		pos = max(0, min(pos, track-1))
		trail := race.Trail(l.trace, track)
		if len(trail) > pos {
			trail = trail[:pos]
		}
		for len(trail) < pos {
			trail = append(trail, 0)
		}

		var b strings.Builder
		b.WriteByte(' ')
		name := layout.Truncate(l.name, maxLaneName)
		b.WriteString(m.paint(l.style, name))
		b.WriteString(strings.Repeat(" ", nameW-layout.Width(name)+2))
		b.WriteString(m.paint(l.style, chart.Scaled(trail, 0, hi)))
		head := "●"
		if l.out {
			head = "×"
		}
		b.WriteString(m.paint(l.style, head))
		if rest := track - pos - 1; rest > 0 {
			b.WriteString(m.paint(m.tbl.Dim, strings.Repeat("·", rest-1)+"│"))
		}

		var right, rstyle string
		switch {
		case l.finished:
			right, rstyle = fmt.Sprintf("%.1fs", l.time.Seconds()), l.style
		case l.out:
			right, rstyle = "out", m.tbl.Dim
		case started:
			right, rstyle = fmt.Sprintf("%s %3.0f", race.Ordinal(rank[l.id]), l.wpm), m.tbl.Dim
		}
		b.WriteString("  ")
		b.WriteString(strings.Repeat(" ", max(0, laneRight-layout.Width(right))))
		b.WriteString(m.paint(rstyle, right))
		out = append(out, b.String())
	}
	return out
}

// header is the title row of every race screen: what is being raced on the
// left, and on the right the room's code, which is what anyone else needs to
// join.
func (m *Model) raceHeader(box int, title string) []string {
	s := m.race
	right := s.st.Code
	if right == "" {
		right = s.st.Room
	}
	return []string{
		m.spread(box, title, right),
		m.paint(m.tbl.Dim, strings.Repeat("─", box)),
		"",
	}
}

// raceRows draws a round: the track, the text with everyone's carets in it,
// and a status line saying where you stand.
func (m *Model) raceRows(box int) []string {
	s := m.race
	now := time.Now()
	title := "race · " + s.st.Setup.Label()
	if s.replay != nil {
		title = fmt.Sprintf("replay · round %d", s.results.Round)
	}
	rows := m.raceHeader(box, title)

	var (
		ls      []lane
		e       = m.eng
		marks   []render.Mark
		hidden  bool
		started bool
	)
	if s.replay != nil {
		ls = m.replayLanes(now)
		started = true
		// Your own run fills the text if you raced; otherwise the winner's.
		view := s.me
		if s.replay.engines[view] == nil && len(s.results.Standings) > 0 {
			view = s.results.Standings[0].ID
		}
		if ve := s.replay.engines[view]; ve != nil {
			e = ve
		}
		lines := m.wrapFor(e, box)
		for id, oe := range s.replay.engines {
			if oe == e || race.Complete(oe) || len(oe.Log) == 0 {
				continue
			}
			_, g := m.racerStyle(id)
			w, c, _ := race.Position(oe)
			if mk, ok := ghostMark(e, lines, w, c, g); ok {
				marks = append(marks, mk)
			}
		}
		// The replay has no live caret, so the run being shown gets a ghost
		// of its own.
		if !race.Complete(e) {
			_, g := m.racerStyle(view)
			w, c, _ := race.Position(e)
			if mk, ok := ghostMark(e, lines, w, c, g); ok {
				marks = append(marks, mk)
			}
		}
	} else {
		ls = m.liveLanes()
		hidden = now.Before(s.goAt)
		started = s.since(now) >= settle
		lines := m.wrapFor(e, box)
		for _, p := range s.st.Players {
			if p.ID == s.me || !p.Racing || p.Finished || p.Out || (p.Word == 0 && p.Char == 0) {
				continue
			}
			_, g := m.racerStyle(p.ID)
			if mk, ok := ghostMark(e, lines, p.Word, p.Char, g); ok {
				marks = append(marks, mk)
			}
		}
	}

	rows = append(rows, m.laneRows(box, ls, started)...)
	rows = append(rows, "")
	caret := s.replay == nil && s.racing && !s.done
	rows = m.textRows(rows, box, e, marks, hidden, caret)
	rows = append(rows, "", m.raceStatus(box, ls, now))
	return rows
}

// raceStatus is the line under the text: the countdown, then your place and
// the time gap to the racer nearest you, then who is still out on the course.
func (m *Model) raceStatus(box int, ls []lane, now time.Time) string {
	s := m.race
	var left, leftPlain, hint string

	switch {
	case s.replay != nil:
		t := s.replay.clock(now)
		leftPlain = fmt.Sprintf("%d× · %.1fs", replaySpeed, t.Seconds())
		left = m.paint(m.tbl.Accent, fmt.Sprintf("%d×", replaySpeed)) + m.paint(m.tbl.Dim, fmt.Sprintf(" · %.1fs", t.Seconds()))
		hint = "r again · esc back"

	case now.Before(s.goAt):
		n := int(math.Ceil(s.goAt.Sub(now).Seconds()))
		leftPlain = fmt.Sprint(n)
		left = m.paint(m.tbl.Accent, leftPlain)
		if now.Sub(s.jumpAt) < jumpFlash {
			left += m.paint(m.tbl.Char[typing.CharIncorrect], "  jump start")
			leftPlain += "  jump start"
		} else {
			left += m.paint(m.tbl.Dim, "  get ready")
			leftPlain += "  get ready"
		}
		hint = "the text opens at zero"

	case s.done:
		rank := ranks(ls)[s.me]
		if s.gaveUp {
			leftPlain = "out"
			left = m.paint(m.tbl.Dim, leftPlain)
		} else {
			leftPlain = fmt.Sprintf("finished %s · %.1fs", race.Ordinal(rank), m.elapsed.Seconds())
			left = m.paint(m.tbl.Accent, "finished "+race.Ordinal(rank)) +
				m.paint(m.tbl.Dim, fmt.Sprintf(" · %.1fs", m.elapsed.Seconds()))
		}
		var waiting []string
		for _, l := range ls {
			if !l.finished && !l.out && l.id != s.me {
				waiting = append(waiting, l.name)
			}
		}
		if len(waiting) > 0 {
			hint = fit(box-layout.Width(leftPlain)-2,
				"waiting for "+strings.Join(waiting, " · "),
				fmt.Sprintf("waiting for %d", len(waiting)))
		}

	default:
		rank := ranks(ls)
		leftPlain = race.Ordinal(rank[s.me])
		left = m.paint(m.tbl.Accent, leftPlain)
		if gap := m.gap(ls, rank, now); gap != "" {
			left += m.paint(m.tbl.Dim, " · "+gap)
			leftPlain += " · " + gap
		}
		hint = "esc give up"
	}

	gap := box - layout.Width(leftPlain) - layout.Width(hint)
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + m.paint(m.tbl.Dim, hint)
}

// gap measures you against the racer nearest you in time: how long ago the one
// ahead was where you are now, or, leading, how long ago you were where the
// one behind is now.
func (m *Model) gap(ls []lane, rank map[int]int, now time.Time) string {
	s := m.race
	if len(ls) < 2 {
		return ""
	}
	me := rank[s.me]
	var you, other lane
	for _, l := range ls {
		switch rank[l.id] {
		case me:
			you = l
		case me - 1:
			other = l
		case me + 1:
			if me == 1 {
				other = l
			}
		}
	}
	if other.id == 0 || other.out {
		return ""
	}
	at := s.since(now)
	if me == 1 {
		t, ok := race.Reached(you.trace, other.done)
		if !ok || other.done == 0 {
			return ""
		}
		return fmt.Sprintf("%.1fs ahead of %s", (at - t).Seconds(), other.name)
	}
	t, ok := race.Reached(other.trace, you.done)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%.1fs behind %s", (at - t).Seconds(), other.name)
}

// lobbyRows draws the room between rounds: who is in it, what the next round
// is, and how to bring someone else in.
func (m *Model) lobbyRows(box int) []string {
	s := m.race
	var rows []string
	if box >= bannerWidth && m.h >= 22 {
		for _, l := range banner {
			rows = append(rows, m.paint(m.tbl.Accent, l))
		}
		rows = append(rows, "")
	}
	rows = append(rows, m.setupBar(box),
		m.paint(m.tbl.Dim, strings.Repeat("─", box)), "")

	for _, p := range s.st.Players {
		fg, _ := m.racerStyle(p.ID)
		line := "  " + m.paint(fg, "●") + " " + m.paint(m.tbl.Text, p.Name)
		var tags []string
		if p.ID == s.me {
			tags = append(tags, "you")
		}
		if p.Host {
			tags = append(tags, "host")
		}
		if p.Wins > 0 {
			tags = append(tags, fmt.Sprintf("%d %s", p.Wins, plural(p.Wins, "win", "wins")))
		}
		if len(tags) > 0 {
			line += m.paint(m.tbl.Dim, "  "+strings.Join(tags, " · "))
		}
		rows = append(rows, line)
	}
	rows = append(rows, "")
	if s.st.Code != "" {
		label, cmd := "  to join: ", "thock race join "+s.st.Code
		if layout.Width(label+cmd) > box {
			label, cmd = "  code: ", s.st.Code
		}
		rows = append(rows, m.paint(m.tbl.Dim, label)+m.paint(m.tbl.Text, cmd))
	} else if s.srv != nil {
		rows = append(rows, m.paint(m.tbl.Dim, fmt.Sprintf("  to join: thock race join <this machine's address>:%d", s.srv.Port())))
	}
	rows = append(rows, "")

	n := len(s.st.Players)
	leftPlain := fmt.Sprintf("%d %s", n, plural(n, "racer", "racers"))
	left := m.paint(m.tbl.Accent, leftPlain)
	var hint string
	switch {
	case s.st.Phase == race.PhaseCountdown || s.st.Phase == race.PhaseRacing:
		hint = fit(box-layout.Width(leftPlain)-2,
			fmt.Sprintf("round %d under way · you race the next one · esc leave", s.st.Round),
			"you race the next round · esc leave",
			"esc leave")
	case s.host():
		hint = fit(box-layout.Width(leftPlain)-2,
			"←→ setup · enter start · esc leave",
			"enter start · esc leave")
	default:
		hint = fit(box-layout.Width(leftPlain)-2,
			"waiting for "+s.hostName()+" to start · esc leave",
			"waiting for "+s.hostName()+" · esc leave",
			"esc leave")
	}
	gap := max(2, box-layout.Width(leftPlain)-layout.Width(hint))
	return append(rows, left+strings.Repeat(" ", gap)+m.paint(m.tbl.Dim, hint))
}

// setupBar shows the race setups the way the config bar shows test settings,
// with the next round's picked out. Narrower boxes drop the word list, then
// every choice but the current one.
func (m *Model) setupBar(box int) string {
	s := m.race
	cur := s.st.Setup
	words := []string{"10", "25", "50", "100"}
	lens := []string{"any", "short", "medium", "long"}
	lists := []string{"1k", "5k"}
	wi, qi := -1, -1
	if cur.Mode == race.ModeQuotes {
		qi = indexOf(lens, cur.Length)
	} else {
		wi = indexOf(words, fmt.Sprint(cur.Words))
	}
	sep := m.paint(m.tbl.Dim, "   ")
	bar := m.choices("words", words, wi) + sep + m.choices("quote", lens, qi)
	full := bar
	if cur.Mode == race.ModeWords {
		full += sep + m.choices("list", lists, indexOf(lists, cur.List))
	}
	for _, v := range []string{full, bar} {
		if layout.Width(stripANSI(v)) <= box {
			return v
		}
	}
	return m.paint(m.tbl.Accent, cur.Label())
}

// podiumRows draws a round's standings, everyone's pace side by side on one
// clock, and the running tally of wins.
func (m *Model) podiumRows(box int) []string {
	s := m.race
	res := s.results
	rows := m.raceHeader(box, fmt.Sprintf("race · round %d · %s", res.Round, s.raced.Label()))

	nameW := 3
	for _, st := range res.Standings {
		nameW = max(nameW, min(layout.Width(m.racerName(st.ID, st.Name)), maxLaneName))
	}

	var winner time.Duration
	if len(res.Standings) > 0 && res.Standings[0].Place == 1 {
		winner = race.Dur(res.Standings[0].Time)
	}
	for _, st := range res.Standings {
		fg, _ := m.racerStyle(st.ID)
		name := layout.Truncate(m.racerName(st.ID, st.Name), maxLaneName)
		place, pstyle := "—", m.tbl.Dim
		if st.Place > 0 {
			place = race.Ordinal(st.Place)
			if st.Place == 1 {
				pstyle = m.tbl.Accent
			}
		}
		line := " " + m.paint(pstyle, fmt.Sprintf("%-4s", place)) + " " +
			m.paint(fg, name) + strings.Repeat(" ", nameW-layout.Width(name)) +
			m.figure(m.tbl.Text, fmt.Sprintf("%.0f", st.WPM), "wpm") +
			m.paint(m.tbl.Dim, fmt.Sprintf("%4.0f%%", st.Accuracy))
		switch {
		case st.Place == 0:
			line += m.paint(m.tbl.Dim, fmt.Sprintf("%9s", "dnf"))
		default:
			line += m.paint(m.tbl.Text, fmt.Sprintf("%8.1fs", race.Dur(st.Time).Seconds()))
			if st.Place > 1 {
				line += m.paint(m.tbl.Dim, fmt.Sprintf("  +%.1fs", (race.Dur(st.Time)-winner).Seconds()))
			}
		}
		if st.ID == s.me && s.isPB {
			line += "  " + m.paint(m.tbl.Accent, "✦ pb")
		}
		rows = append(rows, line)
	}

	if pace := m.paceRows(box, nameW); len(pace) > 0 {
		rows = append(rows, "", m.spread(box, "pace", "raw wpm, one clock for everyone"))
		rows = append(rows, pace...)
	}

	if res.Round > 1 || m.anyWins() {
		var parts []string
		players := append([]race.Player(nil), s.st.Players...)
		sort.SliceStable(players, func(i, j int) bool { return players[i].Wins > players[j].Wins })
		for _, p := range players {
			fg, _ := m.racerStyle(p.ID)
			parts = append(parts, m.paint(fg, m.racerName(p.ID, p.Name))+" "+m.paint(m.tbl.Text, fmt.Sprint(p.Wins)))
		}
		rows = append(rows, "", m.paint(m.tbl.Dim, " wins  ")+strings.Join(parts, m.paint(m.tbl.Dim, " · ")))
	}
	if s.round != nil && s.round.Source != "" {
		rows = append(rows, "", m.paint(m.tbl.Text, " — "+s.round.Source))
	}

	rows = append(rows, "")
	if s.host() {
		rows = append(rows, m.paint(m.tbl.Dim, fit(box,
			" enter rematch · ←→ setup · r replay · ctrl+s stats · esc leave",
			" enter rematch · ←→ setup · r replay · esc leave",
			" enter rematch · r replay · esc leave")))
	} else {
		hint := " r replay · esc leave"
		rows = append(rows, m.spreadDim(box, " next round when "+s.hostName()+" starts", hint))
	}
	return rows
}

func (m *Model) anyWins() bool {
	for _, p := range m.race.st.Players {
		if p.Wins > 0 {
			return true
		}
	}
	return false
}

// paceRows draws each racer's per-second speed against a shared clock and a
// shared scale: a line that ends sooner finished sooner, and a taller one was
// typing faster at that moment.
func (m *Model) paceRows(box, nameW int) []string {
	s := m.race
	cols := box - 1 - nameW - 2 - 1
	if cols < 12 || len(s.pace) == 0 {
		return nil
	}
	longest := 0
	var all []float64
	for _, v := range s.pace {
		longest = max(longest, len(v))
		all = append(all, v...)
	}
	if longest < 2 {
		return nil
	}
	floor, top := chart.Frame(all, 20)

	var out []string
	for _, st := range s.results.Standings {
		v := s.pace[st.ID]
		if len(v) == 0 {
			continue
		}
		fg, _ := m.racerStyle(st.ID)
		name := layout.Truncate(m.racerName(st.ID, st.Name), maxLaneName)
		w := max(1, int(math.Round(float64(len(v))/float64(longest)*float64(cols))))
		line := chart.Scaled(chart.Resample(v, w), floor, top)
		out = append(out, " "+m.paint(fg, name)+strings.Repeat(" ", nameW-layout.Width(name)+2)+m.paint(fg, line))
	}
	return out
}

// browseRows draws the room list.
func (m *Model) browseRows(box int) []string {
	b := m.browse
	var rows []string
	if box >= bannerWidth && m.h >= 22 {
		for _, l := range banner {
			rows = append(rows, m.paint(m.tbl.Accent, l))
		}
		rows = append(rows, "")
	}
	rows = append(rows, m.spread(box, "race", "rooms on your network"),
		m.paint(m.tbl.Dim, strings.Repeat("─", box)), "")

	marker := func(i int) string {
		if b.sel == i && len(b.code) == 0 {
			return m.paint(m.tbl.Accent, "❯ ")
		}
		return "  "
	}
	hostStyle := m.tbl.Dim
	if b.sel == 0 && len(b.code) == 0 {
		hostStyle = m.tbl.Text
	}
	rows = append(rows, marker(0)+m.paint(hostStyle, "+ host a room"))

	nameW, codeW := 4, 4
	for _, f := range b.rooms {
		nameW = max(nameW, layout.Width(f.Room))
		codeW = max(codeW, layout.Width(f.Code))
	}
	for i, f := range b.rooms {
		style := m.tbl.Dim
		if b.sel == i+1 && len(b.code) == 0 {
			style = m.tbl.Text
		}
		state := "in the lobby"
		switch f.Phase {
		case race.PhaseCountdown, race.PhaseRacing:
			state = "racing"
		case race.PhaseResults:
			state = "between rounds"
		}
		label := f.Setup.Label()
		if f.Setup.Mode == "" {
			label = ""
		}
		line := fmt.Sprintf("%-*s  %-*s  %-15s %d %s · %s",
			nameW, f.Room, codeW, f.Code, label, f.Players, plural(f.Players, "racer", "racers"), state)
		rows = append(rows, marker(i+1)+m.paint(style, layout.Truncate(line, box-2)))
	}
	if len(b.rooms) == 0 {
		msg := "listening for rooms…"
		if b.deaf {
			msg = "this network will not let thock listen for rooms; type a code or address"
		}
		rows = append(rows, "  "+m.paint(m.tbl.Dim, layout.Truncate(msg, box-2)))
	}
	rows = append(rows, "")

	label := "  code  "
	field := string(b.code)
	rows = append(rows, m.paint(m.tbl.Dim, label)+m.paint(m.tbl.Text, field))
	if len(b.code) > 0 || b.busy == "" {
		m.caretX = layout.Width(label) + layout.Width(field)
		m.caretY = len(rows) - 1
		m.caretOK = b.busy == ""
	}
	rows = append(rows, "")

	switch {
	case b.busy != "":
		rows = append(rows, m.paint(m.tbl.Accent, "  "+b.busy+"…"))
	case b.notice != "":
		rows = append(rows, m.paint(m.tbl.Char[typing.CharIncorrect], "  "+layout.Truncate(b.notice, box-2)))
	default:
		rows = append(rows, m.paint(m.tbl.Dim, fit(box,
			"  ↑↓ pick · enter join · type a code like velvet-orbit · esc back",
			"  ↑↓ pick · enter join · type a code · esc back",
			"  ↑↓ · enter · esc back")))
	}
	return rows
}

// spreadDim is spread with both sides muted.
func (m *Model) spreadDim(box int, left, right string) string {
	gap := max(2, box-layout.Width(left)-layout.Width(right))
	return m.paint(m.tbl.Dim, left+strings.Repeat(" ", gap)+right)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// stripANSI drops escape sequences, to measure styled text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
