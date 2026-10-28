package dto

import "time"

// MaxRecentPlayers is how many recent players are kept, most recently seen
// first.
const MaxRecentPlayers = 200

// RecentPlayer is someone seen in a lobby or match with the player.
type RecentPlayer struct {
	// Tag is the player's BattleTag.
	Tag string `json:"tag"`
	// Map is the last map played together.
	Map string `json:"map"`
	// LastSeen is the last moment they were in a lobby or match together.
	LastSeen time.Time `json:"lastSeen"`
}
