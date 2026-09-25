package lan

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/dawsonxiong/thock/internal/race"
	"github.com/dawsonxiong/thock/internal/typing"
)

func TestCodeRoundTrip(t *testing.T) {
	local := netip.MustParseAddr("192.168.1.77")
	for _, ap := range []string{"192.168.1.20:47300", "192.168.0.0:47307", "192.168.255.255:47303", "10.0.3.9:47301"} {
		addr := netip.MustParseAddrPort(ap)
		code, err := Code(addr)
		if err != nil {
			t.Fatal(err)
		}
		// A guest on the same network fills in the first two octets itself.
		b := addr.Addr().As4()
		guest := netip.AddrFrom4([4]byte{b[0], b[1], 9, 9})
		if addr.Addr().String() == "192.168.1.20" {
			guest = local
		}
		got, err := Decode(code, guest)
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		if got != addr {
			t.Errorf("%s spelled %s and read back as %s", ap, code, got)
		}
	}
	if _, err := Code(netip.MustParseAddrPort("192.168.1.2:8080")); err == nil {
		t.Error("a port outside the range got a code")
	}
	if _, err := Decode("not-acode", local); err == nil {
		t.Error("nonsense decoded")
	}
	if _, err := Decode("one", local); err == nil {
		t.Error("a single word decoded")
	}
}

func TestResolvePassesAddressesThrough(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.20:47301": "192.168.1.20:47301",
		"192.168.1.20":       "192.168.1.20:47300",
		"localhost:9000":     "localhost:9000",
	} {
		got, err := Resolve(context.Background(), in, nil, 0)
		if err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	known := []Found{{Announcement: Announcement{Code: "velvet-orbit"}, Addr: netip.MustParseAddrPort("10.1.2.3:47302")}}
	if got, _ := Resolve(context.Background(), "Velvet Orbit", known, 0); got != "10.1.2.3:47302" {
		t.Errorf("a heard room was not matched by code: %q", got)
	}
}

// recv waits for the next message of a kind, skipping the rest.
func recv(t *testing.T, c *Client, kind string) race.Msg {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-c.Msgs():
			if !ok {
				t.Fatalf("connection closed waiting for %s: %v", kind, c.Err())
			}
			if m.T == kind {
				return m
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", kind)
		}
	}
}

func typed(words []string) race.Keys {
	var keys race.Keys
	at := time.Duration(0)
	for i, w := range words {
		for _, r := range w {
			at += 5 * time.Millisecond
			keys = append(keys, race.Key{At: at, R: r, Kind: typing.KeyChar})
		}
		if i < len(words)-1 {
			at += 5 * time.Millisecond
			keys = append(keys, race.Key{At: at, R: ' ', Kind: typing.KeySpace})
		}
	}
	return keys
}

// A full round over real sockets: the host's own screen and a guest connect,
// the host starts, both finish, and both get the same results.
func TestRaceOverLoopback(t *testing.T) {
	words := []string{"one", "two", "three"}
	srv, err := Host(HostConfig{
		Name:  "dawson",
		Setup: race.Setup{Mode: race.ModeWords, Words: 10, List: "1k"},
		Text:  func(race.Setup) ([]string, string, error) { return words, "", nil },
		Port:  0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx := context.Background()
	host, err := Dial(ctx, srv.LocalAddr(), "dawson", srv.Token())
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	guest, err := Dial(ctx, srv.LocalAddr(), "maya\x1b[31m", "")
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()

	// A guest cannot start a round.
	guest.Send(race.Msg{T: race.MsgStart})
	st := recv(t, guest, race.MsgState)
	for len(st.State.Players) < 2 {
		st = recv(t, guest, race.MsgState)
	}
	for _, p := range st.State.Players {
		if p.ID == guest.ID() && p.Name != "maya[31m" {
			t.Errorf("guest name not cleaned: %q", p.Name)
		}
	}

	host.Send(race.Msg{T: race.MsgStart})
	s1 := recv(t, host, race.MsgStart).Start
	s2 := recv(t, guest, race.MsgStart).Start
	if s1.Round != 1 || len(s2.Words) != 3 {
		t.Fatalf("start %+v / %+v", s1, s2)
	}
	// Both screens agree on when to go, to within the clock error.
	if d := host.Local(s1.GoAt).Sub(guest.Local(s2.GoAt)); d > 20*time.Millisecond || d < -20*time.Millisecond {
		t.Errorf("start times disagree by %v", d)
	}
	time.Sleep(time.Until(guest.Local(s2.GoAt)) + 200*time.Millisecond)

	keys := typed(words)
	guest.Send(race.Msg{T: race.MsgFinish, Round: 1, Keys: keys})
	time.Sleep(20 * time.Millisecond)
	slower := make(race.Keys, len(keys))
	for i, k := range keys {
		k.At *= 2
		slower[i] = k
	}
	host.Send(race.Msg{T: race.MsgFinish, Round: 1, Keys: slower})

	r1 := recv(t, host, race.MsgResults).Results
	r2 := recv(t, guest, race.MsgResults).Results
	if len(r1.Standings) != 2 || r1.Standings[0].ID != guest.ID() || r1.Standings[0].Place != 1 {
		t.Fatalf("standings %+v", r1.Standings)
	}
	if len(r2.Keys[host.ID()]) != len(keys) {
		t.Error("the guest did not get the host's keys for the replay")
	}
}

func TestHostLeavingClosesTheRoom(t *testing.T) {
	srv, err := Host(HostConfig{Name: "h", Setup: race.Setup{Mode: race.ModeWords, Words: 10, List: "1k"},
		Text: func(race.Setup) ([]string, string, error) { return []string{"a"}, "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	host, err := Dial(context.Background(), srv.LocalAddr(), "h", srv.Token())
	if err != nil {
		t.Fatal(err)
	}
	guest, err := Dial(context.Background(), srv.LocalAddr(), "g", "")
	if err != nil {
		t.Fatal(err)
	}
	host.Close()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-guest.Msgs():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the guest was left in a room with no host")
		}
	}
}

func TestVersionMismatchIsTurnedAway(t *testing.T) {
	srv, err := Host(HostConfig{Name: "h", Setup: race.Setup{Mode: race.ModeWords, Words: 10, List: "1k"},
		Text: func(race.Setup) ([]string, string, error) { return []string{"a"}, "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	conn, err := net.Dial("tcp4", srv.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	b, _ := encode(race.Msg{T: race.MsgHello, V: race.Version + 1, Name: "future"})
	conn.Write(b)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	m, err := newReader(conn).read()
	if err != nil {
		t.Fatal(err)
	}
	if m.T != race.MsgReject || !strings.Contains(m.Reason, "update thock") {
		t.Errorf("got %+v, want a rejection that says what to do", m)
	}
}

// Closing a room while a racer's messages are still arriving must not crash
// the host: a message read just before the close still gets handled, and its
// reply must not go to a connection that is already shut.
func TestCloseWhileMessagesArrive(t *testing.T) {
	for i := 0; i < 30; i++ {
		srv, err := Host(HostConfig{Name: "h", Setup: race.Setup{Mode: race.ModeWords, Words: 10, List: "1k"},
			Text: func(race.Setup) ([]string, string, error) { return []string{"a"}, "", nil }})
		if err != nil {
			t.Fatal(err)
		}
		c, err := Dial(context.Background(), srv.LocalAddr(), "g", "")
		if err != nil {
			srv.Close()
			t.Fatal(err)
		}
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if c.Send(race.Msg{T: race.MsgPing, Sent: 1}) != nil {
					return
				}
			}
		}()
		time.Sleep(time.Millisecond)
		srv.Close()
		close(stop)
		c.Close()
	}
}

// A connection that never says hello, or joins as the room closes, must not
// keep the room from closing: the host's screen waits on it.
func TestCloseDoesNotWaitOnConnections(t *testing.T) {
	srv, err := Host(HostConfig{Name: "h", Setup: race.Setup{Mode: race.ModeWords, Words: 10, List: "1k"},
		Text: func(race.Setup) ([]string, string, error) { return []string{"a"}, "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	silent, err := net.Dial("tcp4", srv.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	guest, err := Dial(context.Background(), srv.LocalAddr(), "g", "")
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		srv.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closing the room waited on a connection")
	}
	if _, err := Dial(context.Background(), srv.LocalAddr(), "late", ""); err == nil {
		t.Error("joined a closed room")
	}
}

// Someone joining between rounds learns what the last round was before they
// are shown its results.
func TestLateJoinerGetsStateBeforeResults(t *testing.T) {
	words := []string{"a", "b"}
	srv, err := Host(HostConfig{Name: "h", Setup: race.Setup{Mode: race.ModeWords, Words: 10, List: "1k"},
		Text: func(race.Setup) ([]string, string, error) { return words, "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	host, err := Dial(context.Background(), srv.LocalAddr(), "h", srv.Token())
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	host.Send(race.Msg{T: race.MsgStart})
	st := recv(t, host, race.MsgStart).Start
	time.Sleep(time.Until(host.Local(st.GoAt)) + 100*time.Millisecond)
	host.Send(race.Msg{T: race.MsgFinish, Round: 1, Keys: typed(words)})
	recv(t, host, race.MsgResults)

	late, err := Dial(context.Background(), srv.LocalAddr(), "late", "")
	if err != nil {
		t.Fatal(err)
	}
	defer late.Close()
	var order []string
	for len(order) < 3 {
		select {
		case m := <-late.Msgs():
			if m.T == race.MsgState || m.T == race.MsgStart || m.T == race.MsgResults {
				order = append(order, m.T)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("got only %v", order)
		}
	}
	if order[0] != race.MsgState || order[1] != race.MsgStart || order[2] != race.MsgResults {
		t.Errorf("late joiner got %v, want state, start, results", order)
	}
}
