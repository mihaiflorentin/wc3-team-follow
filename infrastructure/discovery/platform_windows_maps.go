//go:build windows

package discovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func (windowsPlatform) mapsFolders(_ context.Context, g *game) []dto.MapsFolder {
	var out []dto.MapsFolder
	add := func(base, source string) {
		if base != "" {
			out = append(out, dto.MapsFolder{Path: filepath.Join(base, "Warcraft III", "Maps"), Source: source})
		}
	}
	// The Documents known folder follows OneDrive and moved folders.
	if docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err == nil {
		add(docs, "Your Documents folder")
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		add(filepath.Join(profile, "Documents"), "Your Documents folder")
	}
	if onedrive := os.Getenv("OneDrive"); onedrive != "" {
		add(filepath.Join(onedrive, "Documents"), "OneDrive Documents")
	}
	if g != nil && g.exe != "" {
		// <install>\_retail_\x86_64\Warcraft III.exe: maps next to the game.
		if i := strings.Index(strings.ToLower(g.exe), `\_retail_\`); i > 0 {
			out = append(out, dto.MapsFolder{Path: filepath.Join(g.exe[:i], "Maps"), Source: "Next to the game"})
		}
	}
	return out
}

func (windowsPlatform) gamePath(_ context.Context, _ *game, path string) string { return path }
