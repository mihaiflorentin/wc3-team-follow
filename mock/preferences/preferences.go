// Package preferences holds a test double for contract.PreferenceStore.
package preferences

import (
	"slices"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.PreferenceStore = (*Store)(nil)

// Store keeps preferences in memory.
type Store struct {
	mu      sync.Mutex
	Saved   dto.Preferences
	LoadErr error
	SaveErr error
	Saves   int
}

// New returns a store holding prefs.
func New(prefs dto.Preferences) *Store { return &Store{Saved: prefs} }

func (s *Store) Load() (dto.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.Saved
	out.Tags = slices.Clone(s.Saved.Tags)
	if s.Saved.Host != nil {
		host := *s.Saved.Host
		out.Host = &host
	}
	return out, s.LoadErr
}

func (s *Store) Save(prefs dto.Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Saves++
	s.Saved = prefs
	s.Saved.Tags = slices.Clone(prefs.Tags)
	// Saved settings load as stored, like the real store's file.
	s.Saved.Stored = true
	if prefs.Host != nil {
		host := *prefs.Host
		s.Saved.Host = &host
	}
	return nil
}
