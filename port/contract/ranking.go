package contract

import "github.com/mihaiflorentin/wc3-team-follow/port/dto"

// Ranking keeps the wins and losses recorded for the group, on this
// computer only.
type Ranking interface {
	// Load returns the saved entries. A missing, damaged or oversized file
	// loads as an empty list, not an error.
	Load() ([]dto.RankingEntry, error)
	// Save replaces the saved entries atomically, keeping the
	// dto.MaxRankingEntries most recently played.
	Save(entries []dto.RankingEntry) error
	// Clear removes every saved entry.
	Clear() error
}
