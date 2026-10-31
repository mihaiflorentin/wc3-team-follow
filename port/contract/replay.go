package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// ReplayReader reads Warcraft III replay files (.w3g). It only reads the
// file it is given.
type ReplayReader interface {
	// ReadFile parses the replay at path.
	ReadFile(ctx context.Context, path string) (dto.Replay, error)
}
