package dto

import "time"

// MaxRankingEntries is how many players the ranking keeps, most recently
// played first.
const MaxRankingEntries = 500

// RankingEntry is one player's record in matches played with the group.
// Games and win rate are derived by the window.
type RankingEntry struct {
	// Tag is the player's BattleTag.
	Tag string `json:"tag"`
	// Wins and Losses count the matches whose result was certain.
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	// LastPlayed is when the last counted match ended.
	LastPlayed time.Time `json:"lastPlayed"`
}
