// Package maps lists and searches the Warcraft III maps in a Maps folder.
package maps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	// searchLimit caps search results; searchBudget bounds the walk.
	searchLimit  = 500
	searchBudget = 3 * time.Second
)

// ErrOutsideRoot reports a folder outside the Maps folder.
var ErrOutsideRoot = errors.New("that folder is outside the maps folder")

// downloadCache matches the folders the game names after a map's hash when
// it downloads a map that already exists under the same name.
var downloadCache = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Library implements contract.MapLibrary on the file system.
type Library struct{}

// New returns the map library.
func New() contract.MapLibrary { return Library{} }

// IsMap reports whether name is a Warcraft III map file.
func IsMap(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".w3x" || ext == ".w3m"
}

func mapName(file string) string { return strings.TrimSuffix(file, filepath.Ext(file)) }

func resolve(root, dir string) (string, error) {
	root = filepath.Clean(root)
	full := filepath.Join(root, filepath.FromSlash(dir))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrOutsideRoot
	}
	return full, nil
}

// Browse lists the folders and maps in dir. A download-cache folder that
// holds a single map is shown as that map, so downloads read as maps
// rather than hashes.
func (Library) Browse(_ context.Context, root, dir string) (dto.MapListing, error) {
	full, err := resolve(root, dir)
	if err != nil {
		return dto.MapListing{}, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return dto.MapListing{}, fmt.Errorf("read maps folder: %w", err)
	}
	listing := dto.MapListing{Root: root, Dir: filepath.ToSlash(dir), Entries: []dto.MapEntry{}}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(full, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		switch {
		case e.IsDir() && downloadCache.MatchString(name):
			if only, ok := singleMap(path); ok {
				listing.Entries = append(listing.Entries, only.entry(listing.Dir))
				continue
			}
			listing.Entries = append(listing.Entries, dto.MapEntry{Name: name, IsFolder: true, Path: path, Dir: listing.Dir, Modified: info.ModTime()})
		case e.IsDir():
			listing.Entries = append(listing.Entries, dto.MapEntry{Name: name, IsFolder: true, Path: path, Dir: listing.Dir, Modified: info.ModTime()})
		case IsMap(name):
			listing.Entries = append(listing.Entries, dto.MapEntry{Name: mapName(name), Path: path, Dir: listing.Dir, Modified: info.ModTime(), Size: info.Size()})
		}
	}
	return listing, nil
}

type mapFile struct {
	path string
	info fs.FileInfo
}

func (m mapFile) entry(dir string) dto.MapEntry {
	return dto.MapEntry{Name: mapName(m.info.Name()), Path: m.path, Dir: dir, Modified: m.info.ModTime(), Size: m.info.Size()}
}

// singleMap returns the only map in dir when dir holds exactly one map and
// no folders.
func singleMap(dir string) (mapFile, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return mapFile{}, false
	}
	var found *mapFile
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Name(), "."):
		case e.IsDir():
			return mapFile{}, false
		case IsMap(e.Name()):
			if found != nil {
				return mapFile{}, false
			}
			info, err := e.Info()
			if err != nil {
				return mapFile{}, false
			}
			found = &mapFile{filepath.Join(dir, e.Name()), info}
		}
	}
	if found == nil {
		return mapFile{}, false
	}
	return *found, true
}

// Search walks root for maps whose name contains every word of query,
// ignoring case. Results carry the folder they were found in.
func (Library) Search(ctx context.Context, root, query string) (dto.MapListing, error) {
	words := strings.Fields(strings.ToLower(query))
	listing := dto.MapListing{Root: root, Query: query, Entries: []dto.MapEntry{}}
	if len(words) == 0 {
		return listing, nil
	}
	root = filepath.Clean(root)
	deadline := time.Now().Add(searchBudget)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable folders
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			listing.Truncated = true
			return filepath.SkipAll
		}
		name := d.Name()
		if path != root && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !IsMap(name) {
			return nil
		}
		lower := strings.ToLower(mapName(name))
		for _, w := range words {
			if !strings.Contains(lower, w) {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		if rel == "." {
			rel = ""
		}
		listing.Entries = append(listing.Entries, mapFile{path, info}.entry(filepath.ToSlash(rel)))
		if len(listing.Entries) >= searchLimit {
			listing.Truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return listing, fmt.Errorf("search maps: %w", err)
	}
	return listing, nil
}
