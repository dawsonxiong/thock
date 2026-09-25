package race

import "time"

// Clock maps the host's clock onto a guest's. Each ping measures a round trip
// and the host's time somewhere in the middle of it; the sample with the
// shortest round trip is the one with the least room for error, so it is the
// one kept. On a LAN that is typically within a millisecond or two, which is
// what lets everyone's countdown reach zero together.
type Clock struct {
	offset time.Duration // host minus local
	rtt    time.Duration
	synced bool
}

// Observe records one ping: when it was sent and when the pong came back, on
// the local clock, and the host's clock when it answered.
func (c *Clock) Observe(sent, hostAt, recv time.Duration) {
	rtt := recv - sent
	if rtt < 0 {
		return
	}
	if c.synced && rtt > c.rtt {
		return
	}
	c.rtt = rtt
	c.offset = hostAt + rtt/2 - recv
	c.synced = true
}

// Synced reports whether any ping has come back yet.
func (c *Clock) Synced() bool { return c.synced }

// RTT is the shortest round trip seen.
func (c *Clock) RTT() time.Duration { return c.rtt }

// Local converts a host time to the local clock.
func (c *Clock) Local(host time.Duration) time.Duration { return host - c.offset }
