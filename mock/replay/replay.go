// Package replay holds test doubles for contract.ReplayReader and
// contract.ReplayLocator.
package replay

import (
	"context"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var (
	_ contract.ReplayReader  = (*Reader)(nil)
	_ contract.ReplayLocator = (*Locator)(nil)
)

// Reader returns Replay or Err and records the last path asked for and
// how many files it read.
type Reader struct {
	mu       sync.Mutex
	Replay   dto.Replay
	Err      error
	LastPath string
	reads    int
}

func (r *Reader) ReadFile(_ context.Context, path string) (dto.Replay, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.LastPath = path
	r.reads++
	return r.Replay, r.Err
}

// Set replaces the replay returned from now on.
func (r *Reader) Set(replay dto.Replay) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Replay = replay
}

// Reads returns how many times ReadFile was called.
func (r *Reader) Reads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

// Locator returns File or Err and counts the calls.
type Locator struct {
	mu    sync.Mutex
	File  dto.ReplayFile
	Err   error
	calls int
}

func (l *Locator) LastReplay(context.Context) (dto.ReplayFile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	return l.File, l.Err
}

// Set replaces the file and error returned from now on.
func (l *Locator) Set(file dto.ReplayFile, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.File, l.Err = file, err
}

// Calls returns how many times LastReplay was called.
func (l *Locator) Calls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}
