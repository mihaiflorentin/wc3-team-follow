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

// maxRecentFileSize is far above 200 entries; a larger file is not ours.
const maxRecentFileSize = 512 * 1024

// RecentPlayers keeps the recent players in
// <dir>/settings/recent-players.json, next to friends.json.
type RecentPlayers struct {
	file string
}

// NewRecentPlayers returns a store that keeps its file under dir.
func NewRecentPlayers(dir string) contract.RecentPlayers {
	return &RecentPlayers{file: filepath.Join(dir, "settings", "recent-players.json")}
}

type recentFile struct {
	Players []dto.RecentPlayer `json:"players"`
}

// Load returns the saved list, newest first. A missing, unreadable or
// unrecognized file loads as an empty list: the list is a convenience, and
// the next save replaces it.
func (s *RecentPlayers) Load() ([]dto.RecentPlayer, error) {
	empty := []dto.RecentPlayer{}
	info, err := os.Stat(s.file)
	if err != nil || info.Size() > maxRecentFileSize {
		return empty, nil
	}
	content, err := os.ReadFile(s.file)
	if err != nil {
		return empty, nil
	}
	var saved recentFile
	if err := json.Unmarshal(content, &saved); err != nil {
		return empty, nil
	}
	players := slices.DeleteFunc(saved.Players, func(p dto.RecentPlayer) bool { return strings.TrimSpace(p.Tag) == "" })
	return newest(players), nil
}

// Save replaces the file atomically with the dto.MaxRecentPlayers most
// recently seen players.
func (s *RecentPlayers) Save(players []dto.RecentPlayer) error {
	content, err := json.MarshalIndent(recentFile{Players: newest(slices.Clone(players))}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode recent players: %w", err)
	}
	return writeAtomic(s.file, content)
}

// Clear removes the file.
func (s *RecentPlayers) Clear() error {
	if err := os.Remove(s.file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove recent players: %w", err)
	}
	return nil
}

// newest sorts players most recently seen first and keeps
// dto.MaxRecentPlayers. Never nil.
func newest(players []dto.RecentPlayer) []dto.RecentPlayer {
	if players == nil {
		return []dto.RecentPlayer{}
	}
	slices.SortStableFunc(players, func(a, b dto.RecentPlayer) int { return b.LastSeen.Compare(a.LastSeen) })
	if len(players) > dto.MaxRecentPlayers {
		players = players[:dto.MaxRecentPlayers]
	}
	return players
}
