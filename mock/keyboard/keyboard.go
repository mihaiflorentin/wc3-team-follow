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
// records every line it was asked to type.
type Keyboard struct {
	mu       sync.Mutex
	Current  dto.KeyboardStatus
	Err      error
	PermErr  error
	Lines    []string
	PIDs     []int
	Requests int
}

func (k *Keyboard) Status(context.Context) dto.KeyboardStatus {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.Current
}

func (k *Keyboard) RequestPermission(context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Requests++
	return k.PermErr
}

func (k *Keyboard) TypeLine(_ context.Context, pid int, line string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Lines = append(k.Lines, line)
	k.PIDs = append(k.PIDs, pid)
	return k.Err
}

// Typed returns a copy of the lines typed so far.
func (k *Keyboard) Typed() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.Lines...)
}
