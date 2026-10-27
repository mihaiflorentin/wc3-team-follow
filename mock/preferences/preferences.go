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
	return dto.Preferences{Tags: slices.Clone(s.Saved.Tags), IncludeTeam: s.Saved.IncludeTeam}, s.LoadErr
}

func (s *Store) Save(prefs dto.Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Saves++
	s.Saved = dto.Preferences{Tags: slices.Clone(prefs.Tags), IncludeTeam: prefs.IncludeTeam}
	return nil
}
