package lan

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"sort"
	"time"

	"github.com/dawsonxiong/thock/internal/race"
)

// Group is where rooms announce themselves: an administratively scoped
// multicast address, which routers keep inside the local network.
var Group = netip.MustParseAddrPort("239.255.77.77:47777")

const (
	announceEvery = time.Second
	// forget is how long a room stays listed after its last announcement.
	forget = 3500 * time.Millisecond
	// maxBeacon caps a datagram; a real announcement is a couple of hundred
	// bytes.
	maxBeacon = 2048
)

// Announcement is what a room says about itself once a second.
type Announcement struct {
	T       string     `json:"t"` // always "thock"
	V       int        `json:"v"`
	Room    string     `json:"room"`
	Code    string     `json:"code,omitempty"`
	Port    int        `json:"port"`
	Players int        `json:"players"`
	Phase   race.Phase `json:"phase"`
	Setup   race.Setup `json:"setup"`
}

// Found is a room heard on the network.
type Found struct {
	Announcement
	Addr netip.AddrPort
	Seen time.Time
}

// announce sends the room's announcement until ctx ends. A network that drops
// multicast just means nobody hears it; joining by code or address still works.
func announce(ctx context.Context, what func() Announcement) {
	c, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(Group))
	if err != nil {
		return
	}
	defer c.Close()
	t := time.NewTicker(announceEvery)
	defer t.Stop()
	for {
		a := what()
		a.T, a.V = "thock", race.Version
		if b, err := json.Marshal(a); err == nil {
			c.Write(b)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Browse listens for rooms until ctx ends, sending the full list each time it
// changes. The channel closes when browsing stops. It fails only if the
// network will not let it listen at all.
func Browse(ctx context.Context) (<-chan []Found, error) {
	c, err := net.ListenMulticastUDP("udp4", nil, net.UDPAddrFromAddrPort(Group))
	if err != nil {
		return nil, err
	}
	c.SetReadBuffer(64 << 10)
	out := make(chan []Found, 1)
	go func() {
		<-ctx.Done()
		c.Close()
	}()
	go func() {
		defer close(out)
		rooms := map[netip.AddrPort]Found{}
		buf := make([]byte, maxBeacon)
		for {
			c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, from, err := c.ReadFromUDPAddrPort(buf)
			now := time.Now()
			changed := false
			if err == nil {
				if f, ok := parse(buf[:n], from, now); ok {
					_, known := rooms[f.Addr]
					changed = !known || rooms[f.Addr].Announcement != f.Announcement
					rooms[f.Addr] = f
				}
			} else if ctx.Err() != nil {
				return
			}
			for k, f := range rooms {
				if now.Sub(f.Seen) > forget {
					delete(rooms, k)
					changed = true
				}
			}
			if changed {
				select {
				case out <- list(rooms):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// parse reads one announcement. Everything in it came from the network, so
// the text is cleaned before anything can draw it.
func parse(b []byte, from netip.AddrPort, now time.Time) (Found, bool) {
	var a Announcement
	if json.Unmarshal(b, &a) != nil || a.T != "thock" || a.Port <= 0 || a.Port > 65535 {
		return Found{}, false
	}
	a.Room = race.CleanName(a.Room)
	a.Code = race.CleanCode(a.Code)
	if !a.Setup.Valid() {
		a.Setup = race.Setup{}
	}
	addr := netip.AddrPortFrom(from.Addr().Unmap(), uint16(a.Port))
	return Found{Announcement: a, Addr: addr, Seen: now}, true
}

// list orders rooms by name so the browser does not shuffle as beacons land.
func list(rooms map[netip.AddrPort]Found) []Found {
	out := make([]Found, 0, len(rooms))
	for _, f := range rooms {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Room != out[j].Room {
			return out[i].Room < out[j].Room
		}
		return out[i].Addr.String() < out[j].Addr.String()
	})
	return out
}
