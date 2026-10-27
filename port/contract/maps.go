package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// MapsLocator finds the game's Maps folders and translates paths for the
// game. It knows the running game and, under Wine or Proton, its prefix.
type MapsLocator interface {
	// MapsFolders returns the Maps folders found on this computer, most
	// likely first. It works without the game running, from known places.
	MapsFolders(ctx context.Context) []dto.MapsFolder
	// GamePath returns a map file's path as the game sees it: the same path
	// on Windows and macOS, a Windows path under Wine or Proton.
	GamePath(ctx context.Context, path string) (string, error)
}

// MapLibrary lists and searches the maps in a Maps folder.
type MapLibrary interface {
	// Browse lists one folder: dir is relative to root, "" for root itself.
	Browse(ctx context.Context, root, dir string) (dto.MapListing, error)
	// Search finds maps whose name contains query, in root and below.
	Search(ctx context.Context, root, query string) (dto.MapListing, error)
}
