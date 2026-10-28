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
	// TryReceive returns the next event already received, without
	// waiting; ok is false when none is waiting. It returns an error once
	// the session is closed. Auto-ban reads every waiting event before it
	// bans (docs/adr/0008).
	TryReceive() (ev dto.GameEvent, ok bool, err error)
	Close() error
}

// GameProcesses closes the running Warcraft III clients. It is used only
// when the player asks for it and confirms.
type GameProcesses interface {
	// CloseAll asks every Warcraft III process of this user to close,
	// forces any still running a few seconds later, and returns how many it
	// asked. It touches no other program.
	CloseAll(ctx context.Context) (int, error)
}

// GameCounter is implemented by a discovery error that knows how many
// copies of the game are running, so the companion can report the number
// without parsing the message.
type GameCounter interface {
	GameCount() int
}
