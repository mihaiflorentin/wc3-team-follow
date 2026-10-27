// Package maps holds test doubles for the maps locator and library.
package maps

import (
	"context"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var (
	_ contract.MapsLocator = (*Locator)(nil)
	_ contract.MapLibrary  = (*Library)(nil)
)

// Locator returns fixed folders and prefixes paths with GamePrefix.
type Locator struct {
	Folders    []dto.MapsFolder
	GamePrefix string
}

func (l *Locator) MapsFolders(context.Context) []dto.MapsFolder { return l.Folders }

func (l *Locator) GamePath(_ context.Context, path string) (string, error) {
	return l.GamePrefix + path, nil
}

// Library serves Entries and records the last root it was asked about.
type Library struct {
	Entries  []dto.MapEntry
	LastRoot string
}

func (l *Library) Browse(_ context.Context, root, dir string) (dto.MapListing, error) {
	l.LastRoot = root
	return dto.MapListing{Root: root, Dir: dir, Entries: l.Entries}, nil
}

func (l *Library) Search(_ context.Context, root, query string) (dto.MapListing, error) {
	l.LastRoot = root
	var out []dto.MapEntry
	for _, e := range l.Entries {
		if strings.Contains(strings.ToLower(e.Name), strings.ToLower(query)) {
			out = append(out, e)
		}
	}
	return dto.MapListing{Root: root, Query: query, Entries: out}, nil
}
