// Package race is the pure core of a race between thock players: the messages
// they exchange, the host's room, clock alignment and scoring from keystrokes.
// It does no IO and reads no clock. Every method that cares about time is
// handed it, so a whole race can be played out in a test.
package race

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode"

	"github.com/dawsonxiong/thock/internal/typing"
)

// Version is the wire protocol version. A host turns away a guest speaking a
// different one rather than guessing at what its messages mean.
const Version = 1

// Message kinds. Guests only ever talk to the host, and the host relays what
// everyone needs to see.
const (
	MsgHello    = "hello"    // guest → host: asks to join
	MsgWelcome  = "welcome"  // host → guest: accepted, with the guest's id
	MsgReject   = "reject"   // host → guest: refused, with a reason
	MsgState    = "state"    // host → all: the room as it stands
	MsgSetup    = "setup"    // owner → host: change what the next round types
	MsgStart    = "start"    // owner → host: begin a round; host → all: the text and when to go
	MsgProgress = "progress" // guest → host: where the racer has got to
	MsgFinish   = "finish"   // guest → host: every keystroke, sent once at the line
	MsgForfeit  = "forfeit"  // guest → host: gives up the round
	MsgResults  = "results"  // host → all: standings, and everyone's keystrokes for the replay
	MsgPing     = "ping"     // guest → host: clock probe
	MsgPong     = "pong"     // host → guest: the probe, stamped with the host's clock
)

// Msg is the one envelope every message travels in, one JSON object per line.
// Only the fields a kind needs are set.
type Msg struct {
	T        string    `json:"t"`
	V        int       `json:"v,omitempty"`
	ID       int       `json:"id,omitempty"`
	Name     string    `json:"name,omitempty"`
	Token    string    `json:"token,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Round    int       `json:"round,omitempty"`
	State    *State    `json:"state,omitempty"`
	Setup    *Setup    `json:"setup,omitempty"`
	Start    *Start    `json:"start,omitempty"`
	Progress *Progress `json:"progress,omitempty"`
	Keys     Keys      `json:"keys,omitempty"`
	Results  *Results  `json:"results,omitempty"`
	Sent     int64     `json:"sent,omitempty"` // ping and pong: the guest's clock, ms
	At       int64     `json:"at,omitempty"`   // pong: the host's clock, ms
}

// Phase is where a room is in its cycle.
type Phase string

const (
	PhaseLobby     Phase = "lobby"
	PhaseCountdown Phase = "countdown"
	PhaseRacing    Phase = "racing"
	PhaseResults   Phase = "results"
)

// State is the room as every racer sees it. The host sends it whenever it
// changes, and about ten times a second during a round.
type State struct {
	Room    string   `json:"room"`
	Code    string   `json:"code,omitempty"`
	Phase   Phase    `json:"phase"`
	Round   int      `json:"round"`
	Setup   Setup    `json:"setup"`
	Players []Player `json:"players"`
}

// Player is one racer's public state.
type Player struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Host   bool   `json:"host,omitempty"`
	Wins   int    `json:"wins,omitempty"`
	Racing bool   `json:"racing,omitempty"` // taking part in the current round

	Word     int     `json:"word,omitempty"`
	Char     int     `json:"char,omitempty"`
	Done     float64 `json:"done,omitempty"` // share of the text covered, 0 to 1
	WPM      float64 `json:"wpm,omitempty"`
	Finished bool    `json:"finished,omitempty"`
	Out      bool    `json:"out,omitempty"`  // gave up or left mid-round
	Time     int64   `json:"time,omitempty"` // finish time, ms from the start
}

// Start is a new round: the text and the host-clock time at which it opens.
type Start struct {
	Round  int      `json:"round"`
	Words  []string `json:"words"`
	Source string   `json:"source,omitempty"`
	GoAt   int64    `json:"go"`
}

// Progress is a racer's live position. The host trusts it only for display;
// the result comes from the keystrokes.
type Progress struct {
	Word int     `json:"word"`
	Char int     `json:"char"`
	Done float64 `json:"done"`
	WPM  float64 `json:"wpm"`
}

// Results closes a round.
type Results struct {
	Round     int          `json:"round"`
	Standings []Standing   `json:"standings"`
	Keys      map[int]Keys `json:"keys,omitempty"`
}

// Standing is one racer's line in the results. Place is zero for a racer who
// did not finish.
type Standing struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Place       int     `json:"place"`
	Time        int64   `json:"time,omitempty"`
	WPM         float64 `json:"wpm"`
	Raw         float64 `json:"raw"`
	Accuracy    float64 `json:"acc"`
	Consistency float64 `json:"cons"`
	Done        float64 `json:"done"`
}

// Key is one logged keystroke on the wire. Whether it was correct is not sent:
// the host replays the keys against the text and works that out itself.
type Key struct {
	At   time.Duration
	R    rune
	Kind typing.KeyKind
}

// Keys is a keystroke log, encoded compactly as [ms, rune, kind] triples. A
// 100 word race is around 600 keys, and every racer's log goes to everyone for
// the replay.
type Keys []Key

// MaxKeys bounds a log a guest may send, and MaxKeyTime how late in a round
// a key may be. Both are far beyond any real race. Scoring works second by
// second, so without them one key stamped a year in would make every machine
// that scores it allocate a slot for each of those seconds.
const (
	MaxKeys    = 20000
	MaxKeyTime = time.Hour
)

func (k Keys) MarshalJSON() ([]byte, error) {
	out := make([][3]int64, len(k))
	for i, x := range k {
		out[i] = [3]int64{x.At.Milliseconds(), int64(x.R), int64(x.Kind)}
	}
	return json.Marshal(out)
}

func (k *Keys) UnmarshalJSON(b []byte) error {
	var in [][]int64
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	if len(in) > MaxKeys {
		return fmt.Errorf("keystroke log of %d keys is over the %d limit", len(in), MaxKeys)
	}
	out := make(Keys, len(in))
	for i, x := range in {
		if len(x) != 3 || x[0] < 0 || x[0] > MaxKeyTime.Milliseconds() || x[1] < 0 || x[1] > 0x10FFFF || x[2] < 0 || x[2] > int64(typing.KeyDeleteWord) {
			return errors.New("malformed keystroke")
		}
		out[i] = Key{At: time.Duration(x[0]) * time.Millisecond, R: rune(x[1]), Kind: typing.KeyKind(x[2])}
	}
	*k = out
	return nil
}

// Clean drops keystrokes that could not have come from typing: a character
// that is not printable. Keys typed past the end of a word are drawn in a
// replay, so a log from another machine is cleaned before it is replayed, and
// a host cleans every log before relaying it.
func (k Keys) Clean() Keys {
	out := k[:0:0]
	for _, x := range k {
		if x.Kind == typing.KeyChar && (!unicode.IsPrint(x.R) || unicode.IsSpace(x.R)) {
			continue
		}
		out = append(out, x)
	}
	return out
}

// FromLog converts an engine's keystroke log for sending.
func FromLog(log []typing.Keystroke) Keys {
	out := make(Keys, len(log))
	for i, k := range log {
		out[i] = Key{At: k.At, R: k.R, Kind: k.Kind}
	}
	return out
}

// Ms converts a duration to the protocol's millisecond clock.
func Ms(d time.Duration) int64 { return d.Milliseconds() }

// Dur converts a protocol millisecond time back to a duration.
func Dur(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }
