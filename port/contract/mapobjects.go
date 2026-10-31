package contract

import "context"

// MapObjects reads object names from Warcraft III map files. It only reads
// the file it is given.
type MapObjects interface {
	// Names returns the names of the units, items, upgrades and abilities
	// the map at path defines or changes, by four-character type id.
	Names(ctx context.Context, path string) (map[string]string, error)
}
