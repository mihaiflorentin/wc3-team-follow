// Package ranking holds a test double for contract.Ranking.
package ranking

import (
	"slices"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.Ranking = (*Store)(nil)

// Store keeps the ranking entries in memory and counts writes.
type Store struct {
	mu       sync.Mutex
	saved    []dto.RankingEntry
	SaveErr  error
	ClearErr error
	saves    int
	clears   int
}

// New returns a store holding entries.
func New(entries ...dto.RankingEntry) *Store { return &Store{saved: slices.Clone(entries)} }

func (s *Store) Load() ([]dto.RankingEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.saved), nil
}

func (s *Store) Save(entries []dto.RankingEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.saves++
	s.saved = slices.Clone(entries)
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
func (s *Store) Saved() []dto.RankingEntry {
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

// FailSaves makes Save return err; nil makes it work again.
func (s *Store) FailSaves(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SaveErr = err
}
