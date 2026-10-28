// Package updater holds a test double for contract.Updater. It never
// touches the network.
package updater

import (
	"context"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.Updater = (*Updater)(nil)

// Updater answers Latest with Release and Err and counts the calls. When
// Hold is set, Latest waits until it is closed, as a slow network would.
type Updater struct {
	mu      sync.Mutex
	Release dto.Release
	Err     error
	Hold    chan struct{}
	calls   int
}

// Latest returns the configured release or error.
func (u *Updater) Latest(ctx context.Context) (dto.Release, error) {
	u.mu.Lock()
	u.calls++
	hold := u.Hold
	u.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return dto.Release{}, ctx.Err()
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.Release, u.Err
}

// Set changes the answer.
func (u *Updater) Set(rel dto.Release, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Release, u.Err = rel, err
}

// Calls is how many times Latest was called.
func (u *Updater) Calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls
}
