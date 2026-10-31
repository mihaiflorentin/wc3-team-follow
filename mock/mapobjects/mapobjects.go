// Package mapobjects holds a test double for contract.MapObjects.
package mapobjects

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

var _ contract.MapObjects = (*Reader)(nil)

// Reader returns Result or Err and records the paths asked for.
type Reader struct {
	Result map[string]string
	Err    error
	Paths  []string
}

func (r *Reader) Names(_ context.Context, path string) (map[string]string, error) {
	r.Paths = append(r.Paths, path)
	return r.Result, r.Err
}
