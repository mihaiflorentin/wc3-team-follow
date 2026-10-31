// Package mapdata reads object names (units, items, upgrades, abilities)
// from a Warcraft III map file, so replay orders can show "Bowman" instead
// of h04P. It only reads the file it is given and never changes it.
package mapdata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

// maxMapSize is the largest map file read; Reforged allows 256 MB.
const maxMapSize = 512 << 20

type reader struct{}

// New returns the map object reader.
func New() contract.MapObjects { return reader{} }

// Names returns the custom and changed object names in the map at path, by
// four-character id. Objects the map left as the game made them are not in
// its tables and have no entry.
func (reader) Names(ctx context.Context, path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open map: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxMapSize+1))
	if err != nil {
		return nil, fmt.Errorf("read map: %w", err)
	}
	if len(data) > maxMapSize {
		return nil, fmt.Errorf("map is larger than %d bytes", maxMapSize)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return names(data)
}

// names reads the object names from a map file's bytes.
func names(data []byte) (map[string]string, error) {
	a, err := openArchive(data)
	if err != nil {
		return nil, err
	}
	trig := map[int]string{}
	if wts, err := a.read("war3map.wts"); err == nil {
		trig = mapStrings(wts)
	}
	out := map[string]string{}
	found := false
	for _, f := range objectFiles {
		b, err := a.read(f.name)
		if errors.Is(err, errNoFile) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.name, err)
		}
		found = true
		if err := objectNames(b, f.levels, f.field, trig, out); err != nil {
			return nil, fmt.Errorf("read %s: %w", f.name, err)
		}
	}
	if !found {
		return nil, errors.New("the map has no object tables")
	}
	return out, nil
}
