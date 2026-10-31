package gui

import (
	"errors"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// CheckMaps checks the tracked maps for a newer release now; with Keep
// maps up to date on it also downloads a missing one (ADR 0012).
func (b *Bridge) CheckMaps() (dto.FollowState, error) { return b.companion.CheckMaps() }

// SetMapUpdates switches Keep maps up to date on or off.
func (b *Bridge) SetMapUpdates(on bool) (dto.FollowState, error) {
	return b.companion.Configure(dto.FollowConfig{MapUpdates: &on})
}

// OpenMapReleasePage opens a tracked map's newest release page on GitHub.
// The page passes only the map's name; the address comes from the
// companion's state, and only GitHub's pages over HTTPS are opened.
func (b *Bridge) OpenMapReleasePage(name string) error {
	for _, m := range b.companion.State().MapUpdates.Maps {
		if m.Name == name && m.PageURL != "" {
			return b.open(m.PageURL)
		}
	}
	return errors.New("That map has no release page yet. Check now first.")
}
