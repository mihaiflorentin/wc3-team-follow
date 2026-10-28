// Package keyboard holds a test double for contract.GameKeyboard. It never
// sends a key.
package keyboard

import (
	"context"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.GameKeyboard = (*Keyboard)(nil)

// Keyboard reports Current as its status, fails TypeLine with Err, and
// records every line it was asked to type. When Hold is set, TypeLine
// records the line and then waits until Hold is closed, as a slow typist
// would; it ignores the context so a test can see who waits for it.
type Keyboard struct {
	mu       sync.Mutex
	Current  dto.KeyboardStatus
	Err      error
	PermErr  error
	Hold     chan struct{}
	Lines    []string
	PIDs     []int
	Requests int
}

func (k *Keyboard) Status(context.Context) dto.KeyboardStatus {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.Current
}

// SetStatus changes the status Status reports.
func (k *Keyboard) SetStatus(status dto.KeyboardStatus) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Current = status
}

func (k *Keyboard) RequestPermission(context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Requests++
	return k.PermErr
}

func (k *Keyboard) TypeLine(_ context.Context, pid int, line string) error {
	k.mu.Lock()
	k.Lines = append(k.Lines, line)
	k.PIDs = append(k.PIDs, pid)
	hold, err := k.Hold, k.Err
	k.mu.Unlock()
	if hold != nil {
		<-hold
	}
	return err
}

// Typed returns a copy of the lines typed so far.
func (k *Keyboard) Typed() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.Lines...)
}
