//go:build darwin

package discovery

import (
	"context"
	"os"
	"path/filepath"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func (darwinPlatform) mapsFolders(_ context.Context, _ *game) []dto.MapsFolder {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []dto.MapsFolder{
		{Path: filepath.Join(home, "Library", "Application Support", "Blizzard", "Warcraft III", "Maps"), Source: "Warcraft III's maps folder"},
		{Path: filepath.Join(home, "Documents", "Warcraft III", "Maps"), Source: "Documents"},
	}
}

func (darwinPlatform) replayFolders(_ context.Context, _ *game) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return darwinReplayFolders(home)
}

func (darwinPlatform) gamePath(_ context.Context, _ *game, path string) string { return path }
