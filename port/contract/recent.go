package contract

import "github.com/mihaiflorentin/wc3-team-follow/port/dto"

// RecentPlayers keeps the players the companion saw in a lobby or match
// with the player, on this computer only.
type RecentPlayers interface {
	// Load returns the saved list, newest first. A missing or unreadable
	// file loads as an empty list, not an error.
	Load() ([]dto.RecentPlayer, error)
	// Save replaces the saved list atomically, keeping the
	// dto.MaxRecentPlayers most recently seen.
	Save(players []dto.RecentPlayer) error
	// Clear removes every saved player.
	Clear() error
}
