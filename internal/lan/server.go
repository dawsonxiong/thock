package lan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/dawsonxiong/thock/internal/race"
)

const (
	// helloTimeout is how long a new connection has to say who it is.
	helloTimeout = 5 * time.Second
	// tickEvery is how often the host moves the room along and sends out
	// fresh positions. Ten updates a second is smooth to watch and a few
	// hundred bytes each.
	tickEvery = 100 * time.Millisecond
	// outbox is how many messages may queue for one guest. A guest that
	// falls this far behind is dropped rather than stalling everyone else.
	outbox = 256
)

// TextFunc draws the text for a round.
type TextFunc func(race.Setup) (words []string, source string, err error)

// HostConfig describes a room to open.
type HostConfig struct {
	Name  string     // the room's name, usually the host's
	Setup race.Setup // what the first round types
	Text  TextFunc
	// Port is the port to listen on, or zero to take the first free one of
	// the room code range.
	Port int
	// Beacon turns on the multicast announcement.
	Beacon bool
	// Headless runs a room with no screen of its own: the host's controls
	// go to the first guest, and pass on when they leave.
	Headless bool
}

// Server runs a room. It owns the room state; every racer, the host's own
// screen included, is a client connected to it, so the host's screen goes
// through exactly the same path as everyone else's.
type Server struct {
	ln       net.Listener
	epoch    time.Time
	token    string
	text     TextFunc
	code     string
	headless bool

	mu    sync.Mutex
	room  *race.Room
	peers map[int]*peer
	sent  uint64 // the room version last broadcast

	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

type peer struct {
	id   int
	conn net.Conn
	out  chan []byte
	once sync.Once
}

func (p *peer) close() {
	p.once.Do(func() {
		close(p.out)
		p.conn.Close()
	})
}

// Host opens a room and starts serving it.
func Host(cfg HostConfig) (*Server, error) {
	if cfg.Text == nil {
		return nil, errors.New("a room needs a source of text")
	}
	ln, err := listen(cfg.Port)
	if err != nil {
		return nil, err
	}
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		ln.Close()
		return nil, err
	}

	port := ln.Addr().(*net.TCPAddr).Port
	code := ""
	if ip, ok := LocalIP(); ok {
		code, _ = Code(netip.AddrPortFrom(ip, uint16(port)))
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		ln:       ln,
		epoch:    time.Now(),
		token:    hex.EncodeToString(tok),
		text:     cfg.Text,
		code:     code,
		headless: cfg.Headless,
		room:     race.NewRoom(cfg.Name, code, cfg.Setup),
		peers:    map[int]*peer{},
		cancel:   cancel,
	}
	s.wg.Add(2)
	go s.accept()
	go s.tick(ctx)
	if cfg.Beacon {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			announce(ctx, s.announcement)
		}()
	}
	return s, nil
}

func listen(port int) (net.Listener, error) {
	if port != 0 {
		return net.Listen("tcp4", fmt.Sprintf(":%d", port))
	}
	var err error
	for p := BasePort; p < BasePort+Ports; p++ {
		var ln net.Listener
		if ln, err = net.Listen("tcp4", fmt.Sprintf(":%d", p)); err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port between %d and %d: %w", BasePort, BasePort+Ports-1, err)
}

// Token is the secret that marks a connection as the host's own.
func (s *Server) Token() string { return s.token }

// Code is the room's join code, or empty when this machine has no address
// that can be spelled as one.
func (s *Server) Code() string { return s.code }

// Port is the port the room is listening on.
func (s *Server) Port() int { return s.ln.Addr().(*net.TCPAddr).Port }

// LocalAddr is where a client on this machine connects.
func (s *Server) LocalAddr() string { return fmt.Sprintf("127.0.0.1:%d", s.Port()) }

// Close shuts the room, disconnecting everyone.
func (s *Server) Close() {
	s.once.Do(func() {
		s.cancel()
		s.ln.Close()
		s.mu.Lock()
		for id, p := range s.peers {
			p.close()
			delete(s.peers, id)
		}
		s.mu.Unlock()
		s.wg.Wait()
	})
}

func (s *Server) now() time.Duration { return time.Since(s.epoch) }

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serve(c)
		}()
	}
}

// serve runs one connection from hello to goodbye.
func (s *Server) serve(c net.Conn) {
	r := newReader(c)
	c.SetReadDeadline(time.Now().Add(helloTimeout))
	hello, err := r.read()
	if err != nil || hello.T != race.MsgHello {
		c.Close()
		return
	}
	reject := func(reason string) {
		if b, err := encode(race.Msg{T: race.MsgReject, Reason: reason}); err == nil {
			c.SetWriteDeadline(time.Now().Add(time.Second))
			c.Write(b)
		}
		c.Close()
	}
	if hello.V != race.Version {
		reject(fmt.Sprintf("this room runs race protocol %d and you have %d: update thock on both machines", race.Version, hello.V))
		return
	}
	c.SetReadDeadline(time.Time{})

	s.mu.Lock()
	host := hello.Token != "" && hello.Token == s.token
	if s.headless && !s.room.HasHost() {
		host = true
	}
	id, err := s.room.Join(hello.Name, host)
	if err != nil {
		s.mu.Unlock()
		reject(err.Error())
		return
	}
	p := &peer{id: id, conn: c, out: make(chan []byte, outbox)}
	s.peers[id] = p
	s.wg.Add(1)
	go s.write(p)
	s.send(p, race.Msg{T: race.MsgWelcome, V: race.Version, ID: id})
	// Someone arriving between rounds can replay the last one with everyone
	// else, so they get its text and results too.
	if ph := s.room.Phase(); ph == race.PhaseResults {
		st := s.room.Start()
		s.send(p, race.Msg{T: race.MsgStart, Start: &st})
		s.send(p, race.Msg{T: race.MsgResults, Results: s.room.Results()})
	}
	s.broadcastState()
	s.mu.Unlock()

	for {
		m, err := r.read()
		if err != nil {
			break
		}
		s.mu.Lock()
		s.handle(p, m)
		s.mu.Unlock()
	}

	s.mu.Lock()
	delete(s.peers, id)
	s.room.Leave(id)
	if s.headless {
		s.room.Promote()
	}
	s.broadcastState()
	s.mu.Unlock()
	p.close()

	// A room lives on its host's machine and closes with the host's screen:
	// with nobody able to start a round there would be nothing left to do.
	if host && !s.headless {
		go s.Close()
	}
}

// handle applies one message from a connected player. The lock is held.
func (s *Server) handle(p *peer, m race.Msg) {
	now := s.now()
	switch m.T {
	case race.MsgPing:
		s.send(p, race.Msg{T: race.MsgPong, Sent: m.Sent, At: race.Ms(now)})
	case race.MsgSetup:
		if m.Setup != nil && s.room.SetSetup(p.id, *m.Setup) == nil {
			s.broadcastState()
		}
	case race.MsgStart:
		if !s.room.IsHost(p.id) {
			return
		}
		words, source, err := s.text(s.room.Setup())
		if err != nil {
			return
		}
		st, err := s.room.Begin(p.id, words, source, now)
		if err != nil {
			return
		}
		s.broadcast(race.Msg{T: race.MsgStart, Start: &st})
		s.broadcastState()
	case race.MsgProgress:
		if m.Progress != nil {
			s.room.Report(p.id, m.Round, *m.Progress)
		}
	case race.MsgFinish:
		if s.room.Finish(p.id, m.Round, m.Keys, now) == nil {
			s.broadcastState()
		}
	case race.MsgForfeit:
		if s.room.Forfeit(p.id, m.Round, m.Keys) == nil {
			s.broadcastState()
		}
	}
}

// tick moves the room along the clock and sends out whatever changed.
func (s *Server) tick(ctx context.Context) {
	defer s.wg.Done()
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		if s.room.Tick(s.now()) {
			s.broadcast(race.Msg{T: race.MsgResults, Results: s.room.Results()})
		}
		if s.room.Version() != s.sent {
			s.broadcastState()
		}
		s.mu.Unlock()
	}
}

func (s *Server) broadcastState() {
	st := s.room.State()
	s.broadcast(race.Msg{T: race.MsgState, State: &st})
	s.sent = s.room.Version()
}

func (s *Server) broadcast(m race.Msg) {
	b, err := encode(m)
	if err != nil {
		return
	}
	for _, p := range s.peers {
		s.queue(p, b)
	}
}

func (s *Server) send(p *peer, m race.Msg) {
	if b, err := encode(m); err == nil {
		s.queue(p, b)
	}
}

// queue hands a message to a peer's writer without ever blocking the room.
func (s *Server) queue(p *peer, b []byte) {
	select {
	case p.out <- b:
	default:
		p.conn.Close() // too far behind; its reader will clean up
	}
}

func (s *Server) write(p *peer) {
	defer s.wg.Done()
	for b := range p.out {
		p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := p.conn.Write(b); err != nil {
			p.conn.Close()
			for range p.out {
			}
			return
		}
	}
}

func (s *Server) announcement() Announcement {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Announcement{
		Room:    s.room.Name(),
		Code:    s.code,
		Port:    s.Port(),
		Players: s.room.Count(),
		Phase:   s.room.Phase(),
		Setup:   s.room.Setup(),
	}
}
