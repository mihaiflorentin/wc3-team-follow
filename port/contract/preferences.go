package contract

import "github.com/mihaiflorentin/wc3-team-follow/port/dto"

// PreferenceStore saves the friend selection on this computer.
type PreferenceStore interface {
	// Load returns the saved selection, or the defaults when nothing has
	// been saved yet.
	Load() (dto.Preferences, error)
	// Save validates and stores the selection, replacing it atomically.
	Save(prefs dto.Preferences) error
}
