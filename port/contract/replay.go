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

// ReplayLocator finds the replay the game saves after every match,
// LastReplay.w3g, in the Battle.net account folders (ADR 0013). It only
// looks at folders and file details; it never reads or changes a replay.
type ReplayLocator interface {
	// LastReplay returns the most recently changed LastReplay.w3g across
	// the account folders. It is an error when there is none.
	LastReplay(ctx context.Context) (dto.ReplayFile, error)
}
