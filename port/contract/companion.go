package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Companion is the inbound port the desktop interface drives: it watches
// the game, keeps the follow selection and joins friends' lobbies.
type Companion interface {
	// Run connects to the game and keeps the companion running until ctx
	// is cancelled.
	Run(ctx context.Context) error
	State() dto.FollowState
	Configure(cfg dto.FollowConfig) (dto.FollowState, error)
	// SubmitPassword answers the game's lobby password request once. The
	// password is never stored.
	SubmitPassword(password string) (dto.FollowState, error)
	Diagnostics() dto.Diagnostics
	// Subscribe calls fn with every new state until the returned function
	// is called.
	Subscribe(fn func(dto.FollowState)) (unsubscribe func())
}
