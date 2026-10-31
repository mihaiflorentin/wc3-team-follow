package discovery

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// lastReplayName is the replay the game saves after every match.
const lastReplayName = "LastReplay.w3g"

// ErrNoReplay is returned when no LastReplay.w3g exists.
var ErrNoReplay = errors.New("No replay of your last match was found.")

// NewReplayLocator returns the finder of the game's last replay for this
// operating system (ADR 0013).
func NewReplayLocator(logger *slog.Logger) contract.ReplayLocator {
	return newDiscovery(newPlatform(), logger)
}

// LastReplay returns the most recently changed LastReplay.w3g in the
// account folders. The path holds the Battle.net account id, so only
// whether one was found is logged.
func (d *Discovery) LastReplay(ctx context.Context) (dto.ReplayFile, error) {
	var running *game
	if g, err := oneGame(d.platform.findGames(ctx)); err == nil {
		running = &g
	}
	f, err := newestReplay(d.platform.replayFolders(ctx, running))
	d.logger.DebugContext(ctx, "discovery.last_replay", slog.Bool("found", err == nil))
	return f, err
}

// darwinReplayFolders are the BattleNet folders on macOS, under home.
func darwinReplayFolders(home string) []string {
	return []string{
		filepath.Join(home, "Library", "Application Support", "Warcraft III", "BattleNet"),
		filepath.Join(home, "Library", "Application Support", "Blizzard", "Warcraft III", "BattleNet"),
		filepath.Join(home, "Documents", "Warcraft III", "BattleNet"),
	}
}

// documentsReplayFolders are the BattleNet folders in Documents folders,
// as on Windows: <Documents>/Warcraft III/BattleNet.
func documentsReplayFolders(docs []string) []string {
	var out []string
	for _, d := range docs {
		if d != "" {
			out = append(out, filepath.Join(d, "Warcraft III", "BattleNet"))
		}
	}
	return out
}

// newestReplay looks for <base>/<account>/Replays/LastReplay.w3g in every
// base folder (the game's BattleNet folders, which hold one folder per
// account) and returns the most recently changed one.
func newestReplay(bases []string) (dto.ReplayFile, error) {
	var best dto.ReplayFile
	seen := map[string]bool{}
	for _, base := range bases {
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			path := filepath.Join(base, e.Name(), "Replays", lastReplayName)
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if best.Path == "" || info.ModTime().After(best.Modified) {
				best = dto.ReplayFile{Path: path, Modified: info.ModTime(), Size: info.Size()}
			}
		}
	}
	if best.Path == "" {
		return dto.ReplayFile{}, ErrNoReplay
	}
	return best, nil
}
