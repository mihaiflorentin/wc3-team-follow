// Package replay holds a test double for contract.ReplayReader.
package replay

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.ReplayReader = (*Reader)(nil)

// Reader returns Replay or Err and records the last path asked for.
type Reader struct {
	Replay   dto.Replay
	Err      error
	LastPath string
}

func (r *Reader) ReadFile(_ context.Context, path string) (dto.Replay, error) {
	r.LastPath = path
	return r.Replay, r.Err
}
