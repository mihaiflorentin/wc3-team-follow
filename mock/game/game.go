// Package game holds test doubles for the game discovery, socket and
// process contracts.
package game

import (
	"context"
	"errors"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var (
	_ contract.GameDiscovery = (*Discovery)(nil)
	_ contract.GameDialer    = (*Dialer)(nil)
	_ contract.GameSession   = (*Session)(nil)
	_ contract.GameProcesses = (*Processes)(nil)
	_ contract.GameCounter   = CountedError{}
)

// Processes reports Count closed games, or fails with Err, and counts calls.
// It never touches a real process.
type Processes struct {
	mu    sync.Mutex
	Count int
	Err   error
	Calls int
}

func (p *Processes) CloseAll(context.Context) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Calls++
	return p.Count, p.Err
}

// CallCount returns how many times CloseAll ran.
func (p *Processes) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Calls
}

// CountedError is a discovery error that says how many games are running,
// like discovery's own error for several copies.
type CountedError struct {
	Message string
	Count   int
}

func (e CountedError) Error() string { return e.Message }

func (e CountedError) GameCount() int { return e.Count }

// Discovery returns Conn, or Err when set.
type Discovery struct {
	mu    sync.Mutex
	Conn  dto.GameConnection
	Err   error
	Calls int
}

func (d *Discovery) Discover(context.Context) (dto.GameConnection, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Calls++
	return d.Conn, d.Err
}

// Set changes the result of later calls.
func (d *Discovery) Set(conn dto.GameConnection, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Conn, d.Err = conn, err
}

// Dialer hands out Session, or fails with Err.
type Dialer struct {
	Session *Session
	Err     error
}

func (d *Dialer) Dial(context.Context, dto.GameConnection) (contract.GameSession, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	return d.Session, nil
}

// Sent is one command the session received.
type Sent struct {
	Message string
	Payload map[string]any
}

// Session records sent commands and delivers events pushed with Push.
type Session struct {
	mu     sync.Mutex
	sent   []Sent
	events chan dto.GameEvent
	closed chan struct{}
	once   sync.Once
}

// NewSession returns an open session.
func NewSession() *Session {
	return &Session{events: make(chan dto.GameEvent, 64), closed: make(chan struct{})}
}

func (s *Session) Send(_ context.Context, message string, payload map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closed:
		return errors.New("closed")
	default:
	}
	s.sent = append(s.sent, Sent{message, payload})
	return nil
}

func (s *Session) Receive(ctx context.Context) (dto.GameEvent, error) {
	select {
	case ev := <-s.events:
		return ev, nil
	case <-s.closed:
		return dto.GameEvent{}, errors.New("closed")
	case <-ctx.Done():
		return dto.GameEvent{}, ctx.Err()
	}
}

func (s *Session) TryReceive() (dto.GameEvent, bool, error) {
	select {
	case ev := <-s.events:
		return ev, true, nil
	default:
	}
	select {
	case <-s.closed:
		return dto.GameEvent{}, false, errors.New("closed")
	default:
		return dto.GameEvent{}, false, nil
	}
}

func (s *Session) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

// Push delivers an event to Receive.
func (s *Session) Push(ev dto.GameEvent) { s.events <- ev }

// Sent returns the commands sent so far.
func (s *Session) Sent() []Sent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Sent(nil), s.sent...)
}
