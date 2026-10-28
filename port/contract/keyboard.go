package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// GameKeyboard types one chat line into the running game's match chat with
// keystrokes (ADR 0007). It types only into the given game process, only
// while that process is the frontmost window, and checks that before every
// key; it stops and returns an error the moment focus leaves the game.
type GameKeyboard interface {
	// Status reports whether this build can type on this computer and
	// whether the system permission it needs is granted. It never prompts.
	Status(ctx context.Context) dto.KeyboardStatus
	// RequestPermission asks the system for the permission typing needs,
	// which may show a system prompt. Call it only when the player asks.
	RequestPermission(ctx context.Context) error
	// TypeLine opens the chat with Enter, waits for the chat box, types
	// line and sends it with Enter. The caller passes one trimmed chat
	// line of 1 to 100 printable ASCII characters; anything else is refused
	// whole, without pressing a key.
	TypeLine(ctx context.Context, pid int, line string) error
}
