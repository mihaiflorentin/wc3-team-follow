// Package recent holds a test double for contract.RecentPlayers.
package recent

import (
	"slices"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.RecentPlayers = (*Store)(nil)

// Store keeps the recent players in memory and counts writes.
type Store struct {
	mu       sync.Mutex
	saved    []dto.RecentPlayer
	SaveErr  error
	ClearErr error
	saves    int
	clears   int
}

// New returns a store holding players.
func New(players ...dto.RecentPlayer) *Store { return &Store{saved: slices.Clone(players)} }

func (s *Store) Load() ([]dto.RecentPlayer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.saved), nil
}

func (s *Store) Save(players []dto.RecentPlayer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.saves++
	s.saved = slices.Clone(players)
	return nil
}

func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ClearErr != nil {
		return s.ClearErr
	}
	s.clears++
	s.saved = nil
	return nil
}

// Saved returns the last saved list.
func (s *Store) Saved() []dto.RecentPlayer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.saved)
}

// Saves returns how many times Save succeeded.
func (s *Store) Saves() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves
}

// Clears returns how many times Clear succeeded.
func (s *Store) Clears() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clears
}
