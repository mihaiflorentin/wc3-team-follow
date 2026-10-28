package contract

import "github.com/mihaiflorentin/wc3-team-follow/port/dto"

// PlayerNotes keeps the player's private notes about other players, keyed
// by lower-cased BattleTag, on this computer only.
type PlayerNotes interface {
	// Load returns the saved notes, never nil. A missing or unreadable
	// file loads as no notes, not an error.
	Load() (map[string]dto.PlayerNote, error)
	// Save replaces every saved note atomically.
	Save(notes map[string]dto.PlayerNote) error
}
