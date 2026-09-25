package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/lan"
	"github.com/dawsonxiong/thock/internal/race"
	"github.com/dawsonxiong/thock/internal/typing"
)

var raceWords = []string{"alpha", "beta", "gamma", "delta"}

// newRaceModel opens a room on loopback with a fixed text, seats a model in
// it as the host, and connects a second racer the test drives by hand.
func newRaceModel(t *testing.T) (*Model, *lan.Client) {
	t.Helper()
	isolate(t)
	srv, err := lan.Host(lan.HostConfig{
		Name:  "dawson",
		Setup: race.Setup{Mode: race.ModeWords, Words: 10, List: "1k"},
		Text:  func(race.Setup) ([]string, string, error) { return raceWords, "", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	cl, err := lan.Dial(context.Background(), srv.LocalAddr(), "dawson", srv.Token())
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	guest, err := lan.Dial(context.Background(), srv.LocalAddr(), "maya\x1b[2J", "")
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	m := New(Options{Mode: ModeWords, Duration: 30, Words: 25, List: "1k", Length: "any", Theme: "mono", Colour: false, Name: "dawson"}, config.Default())
	m.Update(tea.WindowSizeMsg{Width: 96, Height: 26})
	m.Join(cl, srv)
	t.Cleanup(func() {
		guest.Close()
		m.Close()
	})
	// The guest's messages are only drained so the host never drops it.
	go func() {
		for range guest.Msgs() {
		}
	}()
	return m, guest
}

// pump feeds the model messages from its room until ok says to stop.
func pump(t *testing.T, m *Model, ok func() bool) {
	t.Helper()
	deadline := time.After(8 * time.Second)
	for !ok() {
		select {
		case msg, open := <-m.race.cl.Msgs():
			if !open {
				t.Fatal("the room closed")
			}
			m.Update(netMsg{m.race, msg})
		case <-deadline:
			t.Fatalf("timed out; screen:\n%s", strip(m.View().Content))
		}
	}
}

func screenText(m *Model) string { return strip(m.View().Content) }

func TestRaceFromLobbyToPodium(t *testing.T) {
	m, guest := newRaceModel(t)
	pump(t, m, func() bool { return len(m.race.st.Players) == 2 })

	lobby := screenText(m)
	for _, want := range []string{"dawson", "maya[2J", "2 racers", "enter start"} {
		if !strings.Contains(lobby, want) {
			t.Errorf("lobby missing %q:\n%s", want, lobby)
		}
	}
	if strings.Contains(m.View().Content, "\x1b[2J") {
		t.Fatal("a player's name cleared the screen")
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pump(t, m, func() bool { return m.screen == screenRace })

	// During the countdown the words are hidden and keys do nothing.
	count := screenText(m)
	if strings.Contains(count, "alpha") || !strings.Contains(count, "▁▁▁▁▁") {
		t.Errorf("text visible before the start:\n%s", count)
	}
	press(m, "alp")
	if len(m.eng.Log) != 0 {
		t.Fatal("a key before the start was typed")
	}
	if !strings.Contains(screenText(m), "jump start") {
		t.Error("a jump start was not called out")
	}

	time.Sleep(time.Until(m.race.goAt) + 10*time.Millisecond)
	m.Update(raceTickMsg{m.race})
	if !strings.Contains(screenText(m), "alpha") {
		t.Fatal("text not revealed at the start")
	}
	// Long enough into the round that the run counts: a run under a second
	// is shown but never recorded.
	time.Sleep(1200 * time.Millisecond)

	// The guest finishes first, typing perfectly.
	var keys race.Keys
	at := time.Duration(0)
	for i, w := range raceWords {
		for _, r := range w {
			at += 10 * time.Millisecond
			keys = append(keys, race.Key{At: at, R: r, Kind: typing.KeyChar})
		}
		if i < len(raceWords)-1 {
			at += 10 * time.Millisecond
			keys = append(keys, race.Key{At: at, R: ' ', Kind: typing.KeySpace})
		}
	}
	guest.Send(race.Msg{T: race.MsgFinish, Round: 1, Keys: keys})

	press(m, strings.Join(raceWords, " "))
	if !m.race.done {
		t.Fatal("typing the whole text did not finish the race")
	}
	if !strings.Contains(screenText(m), "finished") {
		t.Errorf("no finish line status:\n%s", screenText(m))
	}

	pump(t, m, func() bool { return m.screen == screenPodium })
	podium := screenText(m)
	for _, want := range []string{"1st", "2nd", "you", "maya[2J", "pace", "r replay"} {
		if !strings.Contains(podium, want) {
			t.Errorf("podium missing %q:\n%s", want, podium)
		}
	}
	if len(m.records) != 1 || m.records[0].Race == nil || m.records[0].Race.Place != 2 {
		t.Errorf("the run was not recorded as a race: %+v", m.records)
	}

	// The replay plays both runs back from their keys.
	m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.screen != screenRace || m.race.replay == nil {
		t.Fatal("r did not start the replay")
	}
	m.race.replay.start = time.Now().Add(-time.Hour)
	m.Update(raceTickMsg{m.race})
	for id, e := range m.race.replay.engines {
		if !race.Complete(e) {
			t.Errorf("racer %d's replay did not reach the end", id)
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != screenPodium {
		t.Error("esc did not leave the replay")
	}
}

func TestGhostsDoNotChangeTheText(t *testing.T) {
	m, _ := newRaceModel(t)
	pump(t, m, func() bool { return len(m.race.st.Players) == 2 })
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pump(t, m, func() bool { return m.screen == screenRace && allRacing(m) })
	m.race.goAt = time.Now().Add(-2 * time.Second)
	m.Update(raceTickMsg{m.race})

	plain := screenText(m)
	// maya is two letters into "beta".
	for i := range m.race.st.Players {
		p := &m.race.st.Players[i]
		if p.ID != m.race.me {
			p.Word, p.Char, p.Done = 1, 2, 0.4
		}
	}
	raw := m.View().Content
	if !strings.Contains(raw, "\x1b[7m") {
		t.Fatal("no ghost caret drawn")
	}
	if got := screenText(m); textLine(got) != textLine(plain) {
		t.Errorf("a ghost changed the text:\n%s\nvs\n%s", got, plain)
	}

	// At the end of a word the ghost sits on the space after it.
	for i := range m.race.st.Players {
		p := &m.race.st.Players[i]
		if p.ID != m.race.me {
			p.Word, p.Char = 0, len("alpha")
		}
	}
	if !strings.Contains(m.View().Content, "\x1b[7m \x1b[0m") {
		t.Error("a ghost between words is not on the space")
	}
}

func allRacing(m *Model) bool {
	for _, p := range m.race.st.Players {
		if !p.Racing {
			return false
		}
	}
	return len(m.race.st.Players) > 0
}

// textLine is the first line of the text on a race screen.
func textLine(screen string) string {
	for _, l := range strings.Split(screen, "\n") {
		if strings.Contains(l, "alpha") {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

func TestBrowseTakesACode(t *testing.T) {
	isolate(t)
	m := New(Options{Mode: ModeTime, Duration: 30, Words: 25, List: "1k", Length: "any", Theme: "mono"}, config.Default())
	m.Update(tea.WindowSizeMsg{Width: 96, Height: 26})
	ctrl(m, 'r')
	if m.screen != screenBrowse {
		t.Fatal("ctrl+r did not open the room list")
	}
	defer m.Close()
	press(m, "Velvet Orbit!")
	if got := string(m.browse.code); got != "velvet-orbit" {
		t.Errorf("code field holds %q", got)
	}
	if !strings.Contains(screenText(m), "host a room") {
		t.Error("no way to host from the room list")
	}
	key(m, tea.KeyEscape)
	if len(m.browse.code) != 0 {
		t.Error("esc did not clear the code")
	}
	key(m, tea.KeyEscape)
	if m.screen != screenTest || m.browse != nil {
		t.Error("esc did not go back to typing")
	}
}

func TestRaceScreensFitSmallTerminals(t *testing.T) {
	m, _ := newRaceModel(t)
	pump(t, m, func() bool { return len(m.race.st.Players) == 2 })
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pump(t, m, func() bool { return m.screen == screenRace && allRacing(m) })
	m.race.goAt = time.Now().Add(-2 * time.Second)
	m.Update(raceTickMsg{m.race})
	for _, size := range [][2]int{{80, 24}, {60, 20}, {40, 16}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, sc := range []screen{screenLobby, screenBrowse, screenRace} {
			if sc == screenBrowse {
				m.browse = &browser{cancel: func() {}}
			}
			m.screen = sc
			for i, l := range strings.Split(screenText(m), "\n") {
				if w := len([]rune(l)); w > size[0] {
					t.Errorf("%dx%d screen %d line %d is %d wide: %q", size[0], size[1], sc, i, w, l)
				}
			}
		}
		m.browse = nil
	}
}

// TestRaceDump prints race frames without colour, for eyeballing the NO_COLOR
// fallback: go test ./internal/ui -run TestRaceDump -v
func TestRaceDump(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("visual harness; run with -v")
	}
	m, _ := newRaceModel(t)
	pump(t, m, func() bool { return len(m.race.st.Players) == 2 })
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pump(t, m, func() bool { return m.screen == screenRace && allRacing(m) })
	m.race.goAt = time.Now().Add(-2 * time.Second)
	m.Update(raceTickMsg{m.race})
	press(m, "alpha be")
	for i := range m.race.st.Players {
		p := &m.race.st.Players[i]
		if p.ID != m.race.me {
			p.Word, p.Char, p.Done, p.WPM = 2, 3, 0.55, 88
		}
	}
	t.Logf("\n%s", m.View().Content)
}
