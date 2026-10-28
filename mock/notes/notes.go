// Package notes holds a test double for contract.PlayerNotes.
package notes

import (
	"maps"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.PlayerNotes = (*Store)(nil)

// Store keeps the notes in memory and counts writes.
type Store struct {
	mu      sync.Mutex
	saved   map[string]dto.PlayerNote
	SaveErr error
	saves   int
}

// New returns a store holding notes.
func New(notes map[string]dto.PlayerNote) *Store {
	if notes == nil {
		notes = map[string]dto.PlayerNote{}
	}
	return &Store{saved: maps.Clone(notes)}
}

func (s *Store) Load() (map[string]dto.PlayerNote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.saved), nil
}

func (s *Store) Save(notes map[string]dto.PlayerNote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.saves++
	s.saved = maps.Clone(notes)
	return nil
}

// Saved returns the last saved notes.
func (s *Store) Saved() map[string]dto.PlayerNote {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.saved)
}

// Saves returns how many times Save succeeded.
func (s *Store) Saves() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves
}
