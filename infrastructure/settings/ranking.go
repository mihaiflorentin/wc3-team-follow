package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// maxRankingFileSize is far above 2000 entries; a larger file is not ours.
const maxRankingFileSize = 512 * 1024

// Ranking keeps the wins and losses in <dir>/settings/ranking.json, next
// to friends.json.
type Ranking struct {
	file string
}

// NewRanking returns a store that keeps its file under dir.
func NewRanking(dir string) contract.Ranking {
	return &Ranking{file: filepath.Join(dir, "settings", "ranking.json")}
}

type rankingFile struct {
	Entries []dto.RankingEntry `json:"entries"`
}

// Load returns the saved entries, most recently played first. A missing,
// unreadable, oversized or unrecognized file loads as an empty list and is
// left in place: the next save replaces it.
func (s *Ranking) Load() ([]dto.RankingEntry, error) {
	empty := []dto.RankingEntry{}
	info, err := os.Stat(s.file)
	if err != nil || info.Size() > maxRankingFileSize {
		return empty, nil
	}
	content, err := os.ReadFile(s.file)
	if err != nil {
		return empty, nil
	}
	var saved rankingFile
	if err := json.Unmarshal(content, &saved); err != nil {
		return empty, nil
	}
	return mostRecent(cleanRanking(saved.Entries)), nil
}

// Save replaces the file atomically with the dto.MaxRankingEntries most
// recently played entries.
func (s *Ranking) Save(entries []dto.RankingEntry) error {
	content, err := json.MarshalIndent(rankingFile{Entries: mostRecent(cleanRanking(slices.Clone(entries)))}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ranking: %w", err)
	}
	return writeAtomic(s.file, content)
}

// Clear removes the file.
func (s *Ranking) Clear() error {
	if err := os.Remove(s.file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove ranking: %w", err)
	}
	return nil
}

// cleanRanking drops entries without a BattleTag and negative counts.
func cleanRanking(entries []dto.RankingEntry) []dto.RankingEntry {
	entries = slices.DeleteFunc(entries, func(e dto.RankingEntry) bool { return strings.TrimSpace(e.Tag) == "" })
	for i := range entries {
		entries[i].Wins = max(entries[i].Wins, 0)
		entries[i].Losses = max(entries[i].Losses, 0)
	}
	return entries
}

// mostRecent sorts entries most recently played first and keeps
// dto.MaxRankingEntries. Never nil.
func mostRecent(entries []dto.RankingEntry) []dto.RankingEntry {
	if entries == nil {
		return []dto.RankingEntry{}
	}
	slices.SortStableFunc(entries, func(a, b dto.RankingEntry) int { return b.LastPlayed.Compare(a.LastPlayed) })
	if len(entries) > dto.MaxRankingEntries {
		entries = entries[:dto.MaxRankingEntries]
	}
	return entries
}
