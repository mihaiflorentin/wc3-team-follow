package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// GameDiscovery finds the one running Warcraft III client and its local
// menu service. It only reads; it never changes the game.
type GameDiscovery interface {
	Discover(ctx context.Context) (dto.GameConnection, error)
}

// GameDialer opens a session with the game's local menu service.
type GameDialer interface {
	Dial(ctx context.Context, conn dto.GameConnection) (GameSession, error)
}

// GameSession is an open connection to the game's menu service.
type GameSession interface {
	// Send delivers one allow-listed command. Commands outside the
	// allowlist are rejected without being sent.
	Send(ctx context.Context, message string, payload map[string]any) error
	// Receive blocks for the next event. It returns an error once the
	// session is closed.
	Receive(ctx context.Context) (dto.GameEvent, error)
	Close() error
}
