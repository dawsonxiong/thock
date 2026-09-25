package ui

import (
	"context"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/dawsonxiong/thock/internal/lan"
)

const (
	// maxCode bounds the code field; the longest address is well inside it.
	maxCode = 40
	// beaconWait is how long a typed code listens for its room's beacon
	// before working the address out instead.
	beaconWait = 1500 * time.Millisecond
)

// browser is the room list: rooms heard on the network, a row to host one,
// and a field to type a code into.
type browser struct {
	cancel context.CancelFunc
	ch     <-chan []lan.Found
	rooms  []lan.Found
	sel    int // 0 is "host a room", then one row per room
	code   []rune
	busy   string // what is being connected to
	notice string // why the last attempt failed, or the room was lost
	deaf   bool   // the network would not let us listen for rooms
}

type roomsMsg struct {
	b     *browser
	rooms []lan.Found
	ok    bool
}

func (b *browser) listen() tea.Cmd {
	if b.ch == nil {
		return nil
	}
	ch := b.ch
	return func() tea.Msg {
		rooms, ok := <-ch
		return roomsMsg{b, rooms, ok}
	}
}

// Browse opens the model on the room list, as `thock race` does.
func (m *Model) Browse() {
	m.initCmd = m.openBrowse()
}

// openBrowse starts listening for rooms and shows the list.
func (m *Model) openBrowse() tea.Cmd {
	m.stopBrowse()
	ctx, cancel := context.WithCancel(context.Background())
	b := &browser{cancel: cancel}
	ch, err := lan.Browse(ctx)
	if err != nil {
		b.deaf = true
	} else {
		b.ch = ch
	}
	m.browse = b
	m.screen = screenBrowse
	m.overlay = overlayNone
	return b.listen()
}

func (m *Model) stopBrowse() {
	if m.browse != nil {
		m.browse.cancel()
		m.browse = nil
	}
}

func (m *Model) onBrowseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := m.browse
	if b.busy != "" {
		return m, nil // a connection attempt is in flight
	}
	switch msg.String() {
	case "esc":
		if len(b.code) > 0 {
			b.code = b.code[:0]
			return m, nil
		}
		m.stopBrowse()
		m.reset(true)
		return m, nil
	case "up":
		if b.sel > 0 {
			b.sel--
		}
		return m, nil
	case "down":
		if b.sel < len(b.rooms) {
			b.sel++
		}
		return m, nil
	case "backspace":
		if len(b.code) > 0 {
			b.code = b.code[:len(b.code)-1]
		}
		return m, nil
	case "ctrl+w", "ctrl+h", "ctrl+backspace", "alt+backspace":
		b.code = b.code[:0]
		return m, nil
	case "ctrl+s":
		m.openStats()
		return m, nil
	case "enter":
		b.notice = ""
		switch {
		case len(b.code) > 0:
			return m, m.joinRoom(string(b.code))
		case b.sel == 0:
			return m, m.hostRoom()
		case b.sel-1 < len(b.rooms):
			f := b.rooms[b.sel-1]
			return m, m.joinRoom(f.Addr.String())
		}
		return m, nil
	}

	// Anything printable goes into the code field. Codes are words joined by
	// a hyphen, and addresses digits, dots and a colon, so a space is taken
	// as the hyphen it almost certainly means.
	key := msg.Key()
	rs := []rune(key.Text)
	if len(rs) != 1 || unicode.IsControl(rs[0]) || len(b.code) >= maxCode {
		return m, nil
	}
	r := unicode.ToLower(rs[0])
	if r == ' ' {
		r = '-'
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-.:[]", r) {
		b.code = append(b.code, r)
	}
	return m, nil
}

// joinRoom connects to a room by code or address, off the UI goroutine: a
// room that is not there takes a few seconds to give up on.
func (m *Model) joinRoom(target string) tea.Cmd {
	b := m.browse
	b.busy = "joining " + target
	known := b.rooms
	name := m.opts.Name
	return func() tea.Msg {
		ctx := context.Background()
		addr, err := lan.Resolve(ctx, target, known, beaconWait)
		if err != nil {
			return joinedMsg{err: err}
		}
		cl, err := lan.Dial(ctx, addr, name, "")
		if err != nil {
			return joinedMsg{err: err}
		}
		return joinedMsg{s: &session{cl: cl}}
	}
}

// hostRoom opens a room on this machine.
func (m *Model) hostRoom() tea.Cmd {
	m.browse.busy = "opening a room"
	name, setup := m.opts.Name, m.RaceSetup()
	return func() tea.Msg {
		srv, cl, err := HostRace(name, setup)
		if err != nil {
			return joinedMsg{err: err}
		}
		return joinedMsg{s: &session{cl: cl, srv: srv}}
	}
}

// onJoined seats the model in the room it connected to, or reports why not.
func (m *Model) onJoined(msg joinedMsg) tea.Cmd {
	if m.browse == nil || m.screen != screenBrowse {
		// Left the browser while connecting: let the connection go.
		if msg.s != nil {
			msg.s.close()
		}
		return nil
	}
	if msg.err != nil {
		m.browse.busy = ""
		m.browse.notice = msg.err.Error()
		return nil
	}
	m.Join(msg.s.cl, msg.s.srv)
	cmd := m.initCmd
	m.initCmd = nil
	return cmd
}
