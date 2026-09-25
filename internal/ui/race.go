package ui

import (
	"context"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/history"
	"github.com/dawsonxiong/thock/internal/lan"
	"github.com/dawsonxiong/thock/internal/race"
	"github.com/dawsonxiong/thock/internal/stats"
	"github.com/dawsonxiong/thock/internal/typing"
)

const (
	// raceFrame is how often a race redraws on its own, for the countdown,
	// the lanes and the replay. Keystrokes redraw immediately regardless.
	raceFrame = 50 * time.Millisecond
	// reportEvery paces progress reports to the host, which relays them at
	// the same rate.
	reportEvery = 100 * time.Millisecond
	// settle is how long into a round before speeds go on the lanes. In the
	// first second a single fast word reads as an absurd rate.
	settle = time.Second
	// jumpFlash is how long a key pressed before the start is called out.
	jumpFlash = 800 * time.Millisecond
	// replaySpeed plays a round back faster than it happened.
	replaySpeed = 2
)

// session is this screen's seat in a room. The host's screen has one too,
// connected to its own server like everyone else's.
type session struct {
	cl  *lan.Client
	srv *lan.Server // set when this screen is hosting
	me  int

	st      race.State
	round   *race.Start // the text of the current or last round
	results *race.Results

	goAt   time.Time
	racing bool // taking part in the current round
	done   bool // over the line, or out
	gaveUp bool

	reportAt  time.Time
	reportLen int
	liveAt    time.Time
	jumpAt    time.Time

	// traces hold everyone's position over the round, sampled as updates
	// arrive, for the lanes and the time gaps.
	traces map[int][]race.Point

	replay *replay
	pace   map[int][]float64 // per-second speed of each racer, for the podium
	isPB   bool
	// raced is what the last round typed. The podium names it even after the
	// host has picked something else for the next round.
	raced race.Setup
}

// replay plays the last round back from everyone's keystrokes.
type replay struct {
	start   time.Time
	engines map[int]*typing.Engine
	next    map[int]int
	traces  map[int][]race.Point
	end     time.Duration
}

type (
	netMsg struct {
		s *session
		m race.Msg
	}
	netClosedMsg struct{ s *session }
	raceTickMsg  struct{ s *session }
	joinedMsg    struct {
		s   *session
		err error
	}
)

func (s *session) listen() tea.Cmd {
	ch := s.cl.Msgs()
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return netClosedMsg{s}
		}
		return netMsg{s, m}
	}
}

func (s *session) tick() tea.Cmd {
	return tea.Tick(raceFrame, func(time.Time) tea.Msg { return raceTickMsg{s} })
}

func (s *session) close() {
	s.cl.Close()
	if s.srv != nil {
		s.srv.Close()
	}
}

// host reports whether this screen has the host's controls: it is hosting, or
// a headless room handed them over.
func (s *session) host() bool {
	if s.srv != nil {
		return true
	}
	p, ok := s.player(s.me)
	return ok && p.Host
}

// player finds someone in the room.
func (s *session) player(id int) (race.Player, bool) {
	for _, p := range s.st.Players {
		if p.ID == id {
			return p, true
		}
	}
	return race.Player{}, false
}

func (s *session) hostName() string {
	for _, p := range s.st.Players {
		if p.Host {
			return p.Name
		}
	}
	return "the host"
}

// since is time on the round's clock: zero at the start, negative during the
// countdown.
func (s *session) since(now time.Time) time.Duration { return now.Sub(s.goAt) }

// HostRace opens a room on this machine and joins it.
func HostRace(name string, setup race.Setup) (*lan.Server, *lan.Client, error) {
	srv, err := lan.Host(lan.HostConfig{Name: name, Setup: setup, Text: lan.Text, Beacon: true})
	if err != nil {
		return nil, nil, err
	}
	cl, err := lan.Dial(context.Background(), srv.LocalAddr(), name, srv.Token())
	if err != nil {
		srv.Close()
		return nil, nil, err
	}
	return srv, cl, nil
}

// Join seats the model in a room it is already connected to. srv is the room's
// server when this screen is the one hosting it.
func (m *Model) Join(cl *lan.Client, srv *lan.Server) {
	m.stopBrowse()
	s := &session{cl: cl, srv: srv, me: cl.ID(), traces: map[int][]race.Point{}}
	m.race = s
	m.screen = screenLobby
	m.overlay = overlayNone
	m.initCmd = tea.Batch(s.listen(), s.tick())
}

// Close leaves any room and stops listening for rooms. The command calls it
// once the program has ended.
func (m *Model) Close() {
	if m.race != nil {
		m.race.close()
		m.race = nil
	}
	m.stopBrowse()
}

// RaceSetup is the race nearest the solo setup, for a room's first round. A
// timed test has no finish line, so it races the default word count.
func (m *Model) RaceSetup() race.Setup {
	s := race.Setup{Mode: race.ModeWords, Words: m.opts.Words, List: string(m.opts.List)}
	switch m.opts.Mode {
	case ModeQuotes:
		s = race.Setup{Mode: race.ModeQuotes, Length: string(m.opts.Length)}
	case ModeTime:
		s.Words = 25
	}
	if !s.Valid() {
		s = race.Setup{Mode: race.ModeWords, Words: 25, List: "1k"}
	}
	return s
}

// leaveRace drops the room and goes back to looking for one.
func (m *Model) leaveRace(notice string) tea.Cmd {
	if m.race != nil {
		m.race.close()
		m.race = nil
	}
	m.running = false
	m.reset(true)
	cmd := m.openBrowse()
	if m.browse != nil {
		m.browse.notice = notice
	}
	return cmd
}

// onNet applies one message from the host.
func (m *Model) onNet(s *session, msg race.Msg) tea.Cmd {
	now := time.Now()
	switch msg.T {
	case race.MsgState:
		if msg.State == nil {
			break
		}
		st := *msg.State
		st.Room = race.CleanName(st.Room)
		st.Code = race.CleanCode(st.Code)
		if len(st.Players) > race.MaxPlayers {
			st.Players = st.Players[:race.MaxPlayers]
		}
		for i := range st.Players {
			st.Players[i].Name = race.CleanName(st.Players[i].Name)
		}
		s.st = st
		m.traceOthers(s, now)
		if me, ok := s.player(s.me); ok && s.round != nil && st.Round == s.round.Round {
			if !me.Racing && m.screen == screenRace && s.replay == nil {
				s.racing = false
				m.screen = screenLobby
			}
		}

	case race.MsgStart:
		if msg.Start == nil || race.CheckWords(msg.Start.Words) != nil {
			return m.leaveRace("the host sent text thock will not draw")
		}
		st := *msg.Start
		st.Source = race.CleanLine(st.Source)
		m.beginRound(s, st, now)

	case race.MsgResults:
		if msg.Results == nil || s.round == nil || msg.Results.Round != s.round.Round {
			break
		}
		res := *msg.Results
		if len(res.Standings) > race.MaxPlayers {
			res.Standings = res.Standings[:race.MaxPlayers]
		}
		for i := range res.Standings {
			res.Standings[i].Name = race.CleanName(res.Standings[i].Name)
		}
		for id, keys := range res.Keys {
			res.Keys[id] = keys.Clean()
		}
		m.endRound(s, &res)
	}
	return nil
}

// beginRound loads a round's text and starts the countdown to it.
func (m *Model) beginRound(s *session, st race.Start, now time.Time) {
	s.round = &st
	s.results = nil
	s.replay = nil
	s.pace = nil
	s.isPB = false
	s.goAt = s.cl.Local(st.GoAt)
	s.racing = true
	s.done, s.gaveUp = false, false
	s.reportAt, s.reportLen = time.Time{}, 0
	s.traces = map[int][]race.Point{}

	m.eng = typing.New(st.Words)
	m.lines = nil
	m.cache.Invalidate()
	m.running = false
	m.start = s.goAt
	m.elapsed = 0
	m.live = stats.Result{}
	m.res = stats.Result{}
	m.overlay = overlayNone
	m.screen = screenRace
}

// endRound shows the standings, and records this racer's run like any other.
func (m *Model) endRound(s *session, res *race.Results) {
	s.results = res
	s.raced = s.st.Setup
	s.replay = nil
	m.running = false
	m.screen = screenPodium

	s.pace = map[int][]float64{}
	for _, st := range res.Standings {
		keys := res.Keys[st.ID]
		if len(keys) == 0 {
			continue
		}
		e := race.Replay(s.round.Words, keys, keys[len(keys)-1].At)
		r := stats.Compute(e, keys[len(keys)-1].At)
		raw := make([]float64, len(r.Samples))
		for i, x := range r.Samples {
			raw[i] = x.Raw
		}
		s.pace[st.ID] = raw
	}

	for _, st := range res.Standings {
		if st.ID != s.me || st.Place == 0 || stats.TooShort(race.Dur(st.Time)) {
			continue
		}
		rec := history.Record{
			At:          time.Now(),
			Mode:        ModeWords,
			Duration:    int(race.Dur(st.Time).Round(time.Second).Seconds()),
			WPM:         st.WPM,
			Raw:         st.Raw,
			Accuracy:    st.Accuracy,
			Consistency: st.Consistency,
			Chars:       [4]int{m.res.CorrectChars, m.res.IncorrectChars, m.res.ExtraChars, m.res.MissedChars},
			Race:        &history.Race{Place: st.Place, Field: len(res.Standings)},
		}
		if s.raced.Mode == race.ModeQuotes {
			rec.Mode = ModeQuotes
			rec.Source = s.round.Source
		} else {
			rec.Words = s.raced.Words
			rec.List = s.raced.List
		}
		s.isPB = history.IsPB(m.records, rec)
		if err := history.Append(rec); err == nil {
			m.records = append(m.records, rec)
		}
	}
}

// traceOthers samples everyone else's position as an update lands.
func (m *Model) traceOthers(s *session, now time.Time) {
	if s.round == nil || s.st.Round != s.round.Round || s.st.Phase != race.PhaseRacing {
		return
	}
	at := s.since(now)
	if at < 0 {
		return
	}
	for _, p := range s.st.Players {
		if p.ID == s.me || !p.Racing {
			continue
		}
		tr := s.traces[p.ID]
		if n := len(tr); n > 0 && tr[n-1].Done == p.Done {
			continue
		}
		wpm := p.WPM
		if at < settle {
			wpm = 0
		}
		s.traces[p.ID] = append(tr, race.Point{At: at, Done: p.Done, WPM: wpm})
	}
}

// onRaceTick runs the clock: the countdown opening, live figures, reports to
// the host and the replay.
func (m *Model) onRaceTick(s *session) tea.Cmd {
	now := time.Now()
	if s.replay != nil {
		m.stepReplay(s, now)
	}
	if m.screen == screenRace && s.racing && !s.done && s.replay == nil {
		if !m.running && !now.Before(s.goAt) {
			m.running = true
		}
		if m.running {
			m.elapsed = s.since(now)
			if now.Sub(s.liveAt) >= reportEvery {
				s.liveAt = now
				m.live = stats.Compute(m.eng, m.elapsed)
				m.traceMe(s, now)
			}
			m.report(s, now, false)
		}
	}
	return s.tick()
}

// traceMe samples this racer's own position.
func (m *Model) traceMe(s *session, now time.Time) {
	_, _, done := race.Position(m.eng)
	tr := s.traces[s.me]
	if n := len(tr); n > 0 && tr[n-1].Done == done {
		return
	}
	at := s.since(now)
	wpm := m.live.WPM
	if at < settle {
		wpm = 0
	}
	s.traces[s.me] = append(tr, race.Point{At: at, Done: done, WPM: wpm})
}

// report tells the host where this racer is, when there is news.
func (m *Model) report(s *session, now time.Time, force bool) {
	if !force && (now.Sub(s.reportAt) < reportEvery || len(m.eng.Log) == s.reportLen) {
		return
	}
	word, char, done := race.Position(m.eng)
	s.reportAt, s.reportLen = now, len(m.eng.Log)
	s.cl.Send(race.Msg{T: race.MsgProgress, Round: s.round.Round,
		Progress: &race.Progress{Word: word, Char: char, Done: done, WPM: m.live.WPM}})
}

// onRaceKey handles the race screen: typing while the round runs, and the
// replay's controls after it.
func (m *Model) onRaceKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.race
	now := time.Now()
	k := msg.String()

	if s.replay != nil {
		switch k {
		case "esc":
			s.replay = nil
			m.screen = screenPodium
		case "r":
			m.startReplay(s, now)
		}
		return m, nil
	}
	if !s.racing || s.done {
		return m, nil
	}
	if k == "esc" {
		// Giving up still sends the keys, so the replay shows how far you got.
		s.done, s.gaveUp = true, true
		m.running = false
		s.cl.Send(race.Msg{T: race.MsgForfeit, Round: s.round.Round, Keys: race.FromLog(m.eng.Log)})
		return m, nil
	}

	var r rune
	switch k {
	case "ctrl+backspace", "ctrl+h", "ctrl+w", "alt+backspace", "backspace":
	default:
		key := msg.Key()
		rs := []rune(key.Text)
		if len(rs) != 1 || unicode.IsControl(rs[0]) {
			return m, nil
		}
		r = rs[0]
	}
	if now.Before(s.goAt) {
		// A key before zero is a jump start: it is ignored, and said so.
		s.jumpAt = now
		return m, nil
	}
	if !m.running {
		m.running = true
	}
	at := s.since(now)
	switch {
	case k == "backspace":
		m.eng.Backspace(at)
	case r == 0:
		m.eng.DeleteWord(at)
	case r == ' ':
		m.eng.Space(at)
	default:
		m.eng.TypeRune(r, at)
	}

	if race.Complete(m.eng) {
		m.elapsed = at
		m.running = false
		m.res = stats.Compute(m.eng, at)
		m.live = m.res
		m.traceMe(s, now)
		s.done = true
		s.cl.Send(race.Msg{T: race.MsgFinish, Round: s.round.Round, Keys: race.FromLog(m.eng.Log)})
		return m, nil
	}
	m.report(s, now, false)
	return m, nil
}

// onLobbyKey handles the lobby and the podium, which share their controls:
// the host picks the setup and starts, and anyone can leave.
func (m *Model) onLobbyKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.race
	switch msg.String() {
	case "esc":
		return m, m.leaveRace("")
	case "enter":
		if s.host() && s.st.Phase != race.PhaseCountdown && s.st.Phase != race.PhaseRacing {
			s.cl.Send(race.Msg{T: race.MsgStart})
		}
	case "left", "h":
		m.cycleSetup(-1)
	case "right", "l":
		m.cycleSetup(+1)
	case "r":
		if m.screen == screenPodium && s.results != nil {
			m.startReplay(s, time.Now())
		}
	case "ctrl+s":
		m.openStats()
	}
	return m, nil
}

func (m *Model) cycleSetup(delta int) {
	s := m.race
	if !s.host() || s.st.Phase == race.PhaseCountdown || s.st.Phase == race.PhaseRacing {
		return
	}
	list := s.st.Setup.List
	if list == "" {
		list = string(m.opts.List)
	}
	presets := race.Presets(list)
	i := 0
	for j, p := range presets {
		if p == s.st.Setup {
			i = j
		}
	}
	next := presets[(i+delta+len(presets))%len(presets)]
	s.st.Setup = next // shown at once; the host's echo confirms it
	s.cl.Send(race.Msg{T: race.MsgSetup, Setup: &next})
}

// startReplay plays the last round again from everyone's keystrokes.
func (m *Model) startReplay(s *session, now time.Time) {
	if s.results == nil || s.round == nil {
		return
	}
	rp := &replay{start: now, engines: map[int]*typing.Engine{}, next: map[int]int{}, traces: map[int][]race.Point{}}
	for id, keys := range s.results.Keys {
		rp.engines[id] = typing.New(s.round.Words)
		if n := len(keys); n > 0 && keys[n-1].At > rp.end {
			rp.end = keys[n-1].At
		}
	}
	s.replay = rp
	m.screen = screenRace
	m.stepReplay(s, now)
}

// stepReplay moves every racer's replayed engine up to the replay clock.
func (m *Model) stepReplay(s *session, now time.Time) {
	rp := s.replay
	t := min(time.Duration(float64(now.Sub(rp.start))*replaySpeed), rp.end)
	for id, e := range rp.engines {
		keys := s.results.Keys[id]
		before := rp.next[id]
		rp.next[id] = race.Advance(e, keys, before, t)
		if rp.next[id] == before && len(rp.traces[id]) > 0 {
			continue
		}
		_, _, done := race.Position(e)
		wpm := 0.0
		if t >= settle {
			wpm = stats.Compute(e, t).WPM
		}
		rp.traces[id] = append(rp.traces[id], race.Point{At: t, Done: done, WPM: wpm})
	}
}

// replayClock is how far into the round the replay has reached.
func (rp *replay) clock(now time.Time) time.Duration {
	return min(time.Duration(float64(now.Sub(rp.start))*replaySpeed), rp.end)
}
