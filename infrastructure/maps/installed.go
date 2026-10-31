package maps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Pattern compiles a tracked map's file-name pattern and checks that it has
// a group named version.
func Pattern(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("map pattern %q: %w", pattern, err)
	}
	if re.SubexpIndex("version") < 0 {
		return nil, fmt.Errorf("map pattern %q has no group named version", pattern)
	}
	return re, nil
}

// MatchVersion returns the version in a tracked map's file name, or "" when
// name doesn't match the pattern. A name with a path separator never
// matches.
func MatchVersion(pattern, name string) (string, error) {
	re, err := Pattern(pattern)
	if err != nil {
		return "", err
	}
	return matchVersion(re, name), nil
}

func matchVersion(re *regexp.Regexp, name string) string {
	if strings.ContainsAny(name, `/\`) || !IsMap(name) {
		return ""
	}
	m := re.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return m[re.SubexpIndex("version")]
}

// Installed walks root for the tracked map's files, skipping hidden files
// and folders, within the search budget. Most recently changed come first.
func (Library) Installed(ctx context.Context, root string, tracked dto.TrackedMap) ([]dto.InstalledMap, error) {
	re, err := Pattern(tracked.Pattern)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	deadline := time.Now().Add(searchBudget)
	var found []dto.InstalledMap
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil // skip unreadable folders
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return filepath.SkipAll
		}
		name := d.Name()
		if path != root && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		version := matchVersion(re, name)
		if version == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		found = append(found, dto.InstalledMap{Version: version, Path: path, Modified: info.ModTime()})
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("maps folder %s doesn't exist: %w", root, err)
		}
		return nil, fmt.Errorf("find installed maps: %w", err)
	}
	slices.SortStableFunc(found, func(a, b dto.InstalledMap) int { return b.Modified.Compare(a.Modified) })
	return found, nil
}
