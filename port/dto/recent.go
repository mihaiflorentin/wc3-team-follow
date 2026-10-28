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
	// FirstSeen is the first moment they were in a lobby or match together.
	// Lists saved before it existed load with FirstSeen = LastSeen.
	FirstSeen time.Time `json:"firstSeen"`
	// Lobbies is how many distinct lobbies they shared with the player,
	// counted once per lobby, when it starts with them in it or they join.
	// Lists saved before it existed load with 1.
	Lobbies int `json:"lobbies"`
}
