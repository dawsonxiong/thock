package race

import (
	"errors"
	"sort"
	"strconv"
	"time"
)

const (
	// MaxPlayers is the most racers a room holds. Past this the lanes stop
	// fitting on an ordinary terminal.
	MaxPlayers = 8
	// Countdown is how long the text stays hidden after the host starts.
	Countdown = 3 * time.Second
	// minGrace is the least time anyone still racing gets once the first racer
	// is over the line.
	minGrace = 15 * time.Second
	// lateTolerance is how far a racer's own finish time may run past the
	// host's clock before it is disbelieved.
	lateTolerance = 2 * time.Second
)

var (
	ErrFull       = errors.New("the room is full")
	ErrNotHost    = errors.New("only the host can do that")
	ErrBusy       = errors.New("a round is already under way")
	ErrBadSetup   = errors.New("that is not a race setup")
	ErrNoText     = errors.New("there is nothing to type")
	ErrNotRacing  = errors.New("not racing this round")
	ErrUnfinished = errors.New("those keys do not finish the text")
)

// member is a player plus what only the host knows about them.
type member struct {
	Player
	keys Keys
	res  Standing
	left bool // gone from the room, kept only until the round closes
}

// Room is the host's authoritative view of a race. It is not safe for
// concurrent use; the server holds a lock around it.
type Room struct {
	name, code string
	setup      Setup
	phase      Phase
	round      int

	words  []string
	source string
	goAt   time.Duration

	// firstIn is when the first racer crossed the line, on the host's clock,
	// and grace how long the rest then have.
	firstIn time.Duration
	grace   time.Duration

	members []*member
	nextID  int
	results *Results

	// version moves on every change, so the server knows when there is
	// something new to send.
	version uint64
}

// NewRoom opens a room in its lobby.
func NewRoom(name, code string, setup Setup) *Room {
	return &Room{name: CleanName(name), code: code, setup: setup, phase: PhaseLobby, nextID: 1}
}

// Version identifies the room's current state.
func (r *Room) Version() uint64 { return r.version }

// Phase is where the room is in its cycle.
func (r *Room) Phase() Phase { return r.phase }

// Setup is what the next round will type.
func (r *Room) Setup() Setup { return r.setup }

// Round is the number of the current or last round.
func (r *Room) Round() int { return r.round }

// Name is the room's name.
func (r *Room) Name() string { return r.name }

// Code is the room's join code, if it has one.
func (r *Room) Code() string { return r.code }

// Count is how many players are in the room.
func (r *Room) Count() int { return len(r.members) }

func (r *Room) touch() { r.version++ }

func (r *Room) find(id int) *member {
	for _, m := range r.members {
		if m.ID == id {
			return m
		}
	}
	return nil
}

// Join adds a player. A name already in the room gets a number after it, so
// two people called sam can still tell their lanes apart. Someone arriving
// mid-round watches from the lobby and races from the next round.
func (r *Room) Join(name string, host bool) (int, error) {
	if len(r.members) >= MaxPlayers {
		return 0, ErrFull
	}
	name = r.unique(CleanName(name))
	m := &member{Player: Player{ID: r.nextID, Name: name, Host: host}}
	r.nextID++
	r.members = append(r.members, m)
	r.touch()
	return m.ID, nil
}

func (r *Room) unique(name string) string {
	taken := func(n string) bool {
		for _, m := range r.members {
			if m.Name == n {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		n := name + strconv.Itoa(i)
		if !taken(n) {
			return n
		}
	}
}

// Leave removes a player. Someone who leaves mid-round keeps their lane,
// marked out, so the standings still show they were there.
func (r *Room) Leave(id int) {
	for i, m := range r.members {
		if m.ID != id {
			continue
		}
		if m.Racing && (r.phase == PhaseCountdown || r.phase == PhaseRacing) && !m.Finished {
			m.Out = true
			m.left = true
		} else {
			r.members = append(r.members[:i], r.members[i+1:]...)
		}
		r.touch()
		return
	}
}

// IsHost reports whether a player may start rounds and change the setup.
func (r *Room) IsHost(id int) bool {
	m := r.find(id)
	return m != nil && m.Host
}

// HasHost reports whether anyone in the room may start rounds.
func (r *Room) HasHost() bool {
	for _, m := range r.members {
		if m.Host && !m.left {
			return true
		}
	}
	return false
}

// Promote hands the host's controls to the longest-standing player when
// nobody has them. A room with no screen of its own uses it, so there is
// always someone who can start the next round.
func (r *Room) Promote() {
	if r.HasHost() {
		return
	}
	for _, m := range r.members {
		if !m.left {
			m.Host = true
			r.touch()
			return
		}
	}
}

// SetSetup changes what the next round types. Only the host may, and not while
// a round is running.
func (r *Room) SetSetup(id int, s Setup) error {
	m := r.find(id)
	if m == nil || !m.Host {
		return ErrNotHost
	}
	if r.phase == PhaseCountdown || r.phase == PhaseRacing {
		return ErrBusy
	}
	if !s.Valid() {
		return ErrBadSetup
	}
	r.setup = s
	r.touch()
	return nil
}

// Begin starts a round over the given text. Everyone in the room races it, and
// the text opens after the countdown.
func (r *Room) Begin(id int, words []string, source string, now time.Duration) (Start, error) {
	m := r.find(id)
	if m == nil || !m.Host {
		return Start{}, ErrNotHost
	}
	if r.phase == PhaseCountdown || r.phase == PhaseRacing {
		return Start{}, ErrBusy
	}
	if len(words) == 0 {
		return Start{}, ErrNoText
	}
	r.round++
	r.words = words
	r.source = source
	r.goAt = now + Countdown
	r.firstIn, r.grace = 0, 0
	r.results = nil
	r.phase = PhaseCountdown
	for _, m := range r.members {
		m.Player = Player{ID: m.ID, Name: m.Name, Host: m.Host, Wins: m.Wins, Racing: true}
		m.keys = nil
		m.res = Standing{}
	}
	r.touch()
	return r.Start(), nil
}

// Start describes the current round, for anyone who needs its text.
func (r *Room) Start() Start {
	return Start{Round: r.round, Words: r.words, Source: r.source, GoAt: Ms(r.goAt)}
}

// racer finds a player taking part in the running round.
func (r *Room) racer(id, round int) (*member, error) {
	m := r.find(id)
	if m == nil || !m.Racing || m.Out || round != r.round {
		return nil, ErrNotRacing
	}
	if r.phase != PhaseRacing && r.phase != PhaseCountdown {
		return nil, ErrNotRacing
	}
	return m, nil
}

// Report updates a racer's live position.
func (r *Room) Report(id, round int, p Progress) error {
	m, err := r.racer(id, round)
	if err != nil || m.Finished {
		return err
	}
	m.Word, m.Char = max(0, p.Word), max(0, p.Char)
	m.Done = min(max(p.Done, 0), 0.999) // only the keys can say it is finished
	m.WPM = min(max(p.WPM, 0), 999)
	r.touch()
	return nil
}

// Finish takes a racer's keystrokes at the line. The host replays them against
// the text rather than trusting a reported score, so every result is worked
// out the same way on the same machine.
func (r *Room) Finish(id, round int, keys Keys, now time.Duration) error {
	m, err := r.racer(id, round)
	if err != nil {
		return err
	}
	if m.Finished {
		return nil
	}
	keys = keys.Clean()
	res, finished := Score(r.words, keys)
	if !finished {
		return ErrUnfinished
	}
	took := keys[len(keys)-1].At
	// A finish time later than the host's own clock allows is not believable,
	// and one before the start is not possible.
	if took <= 0 || r.goAt+took > now+lateTolerance {
		return ErrUnfinished
	}
	m.Finished = true
	m.Done = 1
	m.Time = Ms(took)
	m.WPM = res.WPM
	m.keys = keys
	m.res = Standing{
		ID: m.ID, Name: m.Name, Time: Ms(took), Done: 1,
		WPM: res.WPM, Raw: res.Raw, Accuracy: res.Accuracy, Consistency: res.Consistency,
	}
	if r.firstIn == 0 {
		r.firstIn = now
		// The field gets as long again as the winner took, so anyone at half
		// the winner's speed still finishes, and never less than minGrace.
		r.grace = max(minGrace, took)
	}
	r.touch()
	return nil
}

// Forfeit takes a racer out of the running round, keeping what they typed so
// far so their keys can still be replayed.
func (r *Room) Forfeit(id, round int, keys Keys) error {
	m, err := r.racer(id, round)
	if err != nil {
		return err
	}
	if m.Finished {
		return nil
	}
	m.Out = true
	m.keys = keys.Clean()
	r.touch()
	return nil
}

// Tick moves the room along the clock: the countdown opens into the race, and
// the race closes once everyone is in or out, or the grace after the first
// finish runs out. It reports whether the round just ended.
func (r *Room) Tick(now time.Duration) bool {
	switch r.phase {
	case PhaseCountdown:
		if now >= r.goAt {
			r.phase = PhaseRacing
			r.touch()
		}
	case PhaseRacing:
		if r.allIn() || (r.firstIn > 0 && now >= r.firstIn+r.grace) {
			r.close()
			return true
		}
	}
	return false
}

func (r *Room) allIn() bool {
	for _, m := range r.members {
		if m.Racing && !m.Finished && !m.Out {
			return false
		}
	}
	return true
}

// close ranks the round: finishers by time, then everyone else by how far
// they got.
func (r *Room) close() {
	res := &Results{Round: r.round, Keys: map[int]Keys{}}
	for _, m := range r.members {
		if !m.Racing {
			continue
		}
		s := m.res
		if !m.Finished {
			s = Standing{ID: m.ID, Name: m.Name, Done: m.Done, WPM: m.WPM}
			// The keys say how far they really got, where the last report
			// may be a moment stale.
			if len(m.keys) > 0 {
				end := m.keys[len(m.keys)-1].At
				e := Replay(r.words, m.keys, end)
				_, _, s.Done = Position(e)
			}
		}
		res.Standings = append(res.Standings, s)
		if len(m.keys) > 0 {
			res.Keys[m.ID] = m.keys
		}
	}
	sort.SliceStable(res.Standings, func(i, j int) bool {
		a, b := res.Standings[i], res.Standings[j]
		if (a.Time > 0) != (b.Time > 0) {
			return a.Time > 0
		}
		if a.Time > 0 {
			return a.Time < b.Time
		}
		return a.Done > b.Done
	})
	for i := range res.Standings {
		if res.Standings[i].Time > 0 {
			res.Standings[i].Place = i + 1
		}
	}
	if len(res.Standings) > 0 && res.Standings[0].Place == 1 {
		if w := r.find(res.Standings[0].ID); w != nil {
			w.Wins++
		}
	}
	// Anyone who left mid-round has their line in the standings now, and
	// nothing keeps them in the room.
	kept := r.members[:0]
	for _, m := range r.members {
		if !m.left {
			kept = append(kept, m)
		}
	}
	r.members = kept
	r.results = res
	r.phase = PhaseResults
	r.touch()
}

// Results is the last closed round, or nil.
func (r *Room) Results() *Results { return r.results }

// State is the room as the players see it.
func (r *Room) State() State {
	s := State{Room: r.name, Code: r.code, Phase: r.phase, Round: r.round, Setup: r.setup}
	s.Players = make([]Player, len(r.members))
	for i, m := range r.members {
		s.Players[i] = m.Player
	}
	return s
}
