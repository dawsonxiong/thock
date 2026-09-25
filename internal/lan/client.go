package lan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/dawsonxiong/thock/internal/race"
)

const (
	dialTimeout = 3 * time.Second
	// The first few pings go quickly so the clock is aligned well before
	// anyone can press start, then settle to one a second.
	fastPings = 5
	fastEvery = 100 * time.Millisecond
	pingEvery = time.Second
)

// Client is one racer's connection to a room.
type Client struct {
	conn  net.Conn
	epoch time.Time
	id    int
	in    chan race.Msg

	wmu sync.Mutex // serialises writes

	mu    sync.Mutex
	clock race.Clock
	err   error

	done chan struct{}
	once sync.Once
}

// Dial joins the room at addr. token is empty for a guest, and the server's
// token for the host's own screen.
func Dial(ctx context.Context, addr, name, token string) (*Client, error) {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp4", addr)
	if err != nil {
		return nil, fmt.Errorf("no room answered at %s", addr)
	}
	c := &Client{conn: conn, epoch: time.Now(), in: make(chan race.Msg, 64), done: make(chan struct{})}
	if err := c.Send(race.Msg{T: race.MsgHello, V: race.Version, Name: name, Token: token}); err != nil {
		conn.Close()
		return nil, err
	}

	r := newReader(conn)
	conn.SetReadDeadline(time.Now().Add(helloTimeout))
	m, err := r.read()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("the room at %s did not answer: %w", addr, err)
	}
	conn.SetReadDeadline(time.Time{})
	switch m.T {
	case race.MsgWelcome:
		c.id = m.ID
	case race.MsgReject:
		conn.Close()
		return nil, fmt.Errorf("the room turned you away: %s", race.CleanLine(m.Reason))
	default:
		conn.Close()
		return nil, errors.New("that is not a thock room")
	}

	go c.read(r)
	go c.ping()
	return c, nil
}

// ID is this racer's id in the room.
func (c *Client) ID() int { return c.id }

// Msgs delivers messages from the host. It closes when the connection ends,
// after which Err says why.
func (c *Client) Msgs() <-chan race.Msg { return c.in }

// Err is why the connection ended, or nil while it is up.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Send writes one message to the host.
func (c *Client) Send(m race.Msg) error {
	b, err := encode(m)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = c.conn.Write(b)
	return err
}

// Close leaves the room.
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
		c.conn.Close()
	})
}

// Local converts a time on the host's clock to local wall time.
func (c *Client) Local(hostMs int64) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch.Add(c.clock.Local(race.Dur(hostMs)))
}

// RTT is the best round trip to the host so far.
func (c *Client) RTT() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clock.RTT()
}

func (c *Client) read(r *reader) {
	defer close(c.in)
	for {
		m, err := r.read()
		if err != nil {
			c.mu.Lock()
			if c.err == nil {
				c.err = err
			}
			c.mu.Unlock()
			c.Close()
			return
		}
		if m.T == race.MsgPong {
			// Pongs are for the clock, not for the screen.
			recv := time.Since(c.epoch)
			c.mu.Lock()
			c.clock.Observe(race.Dur(m.Sent), race.Dur(m.At), recv)
			c.mu.Unlock()
			continue
		}
		select {
		case c.in <- m:
		case <-c.done:
			return
		}
	}
}

func (c *Client) ping() {
	for i := 0; ; i++ {
		// A probe stamped with zero would be dropped by omitempty, so the
		// clock is read after at least a millisecond has passed.
		sent := max(time.Since(c.epoch), time.Millisecond)
		if c.Send(race.Msg{T: race.MsgPing, Sent: race.Ms(sent)}) != nil {
			return
		}
		wait := pingEvery
		if i < fastPings {
			wait = fastEvery
		}
		select {
		case <-c.done:
			return
		case <-time.After(wait):
		}
	}
}

// Resolve turns what someone typed to join a room into an address. An address
// with a port is used as it is. A room code is looked up among rooms already
// found, then by listening for its beacon for up to wait, and failing that
// worked out from this machine's own address.
func Resolve(ctx context.Context, target string, known []Found, wait time.Duration) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("give a room code, like velvet-orbit, or an address, like 192.168.1.20:47300")
	}
	if _, _, err := net.SplitHostPort(target); err == nil {
		return target, nil
	}
	if ip, err := netip.ParseAddr(target); err == nil {
		return netip.AddrPortFrom(ip, BasePort).String(), nil
	}
	code := strings.ToLower(strings.ReplaceAll(target, " ", "-"))
	for _, f := range known {
		if f.Code == code {
			return f.Addr.String(), nil
		}
	}
	if wait > 0 {
		ctx, cancel := context.WithTimeout(ctx, wait)
		defer cancel()
		if ch, err := Browse(ctx); err == nil {
			for rooms := range ch {
				for _, f := range rooms {
					if f.Code == code {
						cancel()
						return f.Addr.String(), nil
					}
				}
			}
		}
	}
	local, ok := LocalIP()
	if !ok {
		return "", errors.New("this machine has no network address to find the room from")
	}
	ap, err := Decode(code, local)
	if err != nil {
		return "", err
	}
	return ap.String(), nil
}
