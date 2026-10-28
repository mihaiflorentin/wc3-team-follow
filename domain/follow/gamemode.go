package follow

import (
	"errors"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	// DefaultGameModeDelay is Game mode's delay, in seconds, when none was
	// saved.
	DefaultGameModeDelay = dto.DefaultGameModeDelaySeconds
	// MaxGameModeDelay is the longest Game mode delay, in seconds.
	MaxGameModeDelay = dto.MaxGameModeDelaySeconds
)

// gameModeState is Game mode's progress in the current hosted match.
type gameModeState struct {
	// at is when the command is due; zero until the match starts.
	at time.Time
	// done is set once the command was requested, or cancelled, this match.
	done bool
}

// normalizeGameMode trims the Game mode command and keeps its delay within
// 0 to MaxGameModeDelay seconds. An empty command turns Game mode off.
func normalizeGameMode(command string, delay int) (string, int, error) {
	delay = min(max(delay, 0), MaxGameModeDelay)
	command = strings.TrimSpace(command)
	if command == "" {
		return "", delay, nil
	}
	line, err := ChatLine(command)
	if err != nil {
		return "", delay, errors.New("The game mode command must be one line of up to 100 characters.")
	}
	return line, delay, nil
}

// gameModeReceive follows the start and end of a match from a lobby
// Auto-host created. The match starts with SetGlueScreen DISABLED_SCREEN
// and IsGameUIActive true once every player has loaded (live check, issue
// 9); LoadingScreenGameStart never arrives.
//
// Only the real end of the match cancels a command that isn't due yet:
// ExitedGame or a score screen (handled with Auto-host's phase in
// host.go), MultiplayerGameLeave here, or a disconnect, which resets
// Auto-host. IsGameUIActive false doesn't: it may come from alt-tab or
// the in-game menu.
func (e *Engine) gameModeReceive(kind string, p map[string]any) {
	h := &e.host
	if !h.enabled || h.phase != HostMatch || h.settings.GameMode == "" {
		return
	}
	switch kind {
	case "SetGlueScreen":
		if screen, _ := p["screen"].(string); screen == "DISABLED_SCREEN" {
			e.gameModeStart()
		}
	case "IsGameUIActive":
		if active := p["isActive"]; active == true || active == "true" {
			e.gameModeStart()
		}
	case "MultiplayerGameLeave":
		e.gameModeCancel()
	}
}

func (e *Engine) gameModeStart() {
	g, s := &e.host.gameMode, e.host.settings
	if g.done || !g.at.IsZero() {
		return
	}
	g.at = e.now().Add(time.Duration(s.GameModeDelaySeconds) * time.Second)
	if s.GameModeDelaySeconds > 0 {
		e.Note("The match started. Game mode types " + s.GameMode + " in " + itoa(s.GameModeDelaySeconds) +
			" seconds; keep Warcraft III in front.")
	}
}

// gameModeCancel drops a command that isn't due yet: the match ended or
// the player left first.
func (e *Engine) gameModeCancel() {
	g := &e.host.gameMode
	if g.done || g.at.IsZero() {
		return
	}
	g.at, g.done = time.Time{}, true
	e.Note("The match ended before Game mode typed " + e.host.settings.GameMode + ".")
}

// gameModeStep asks for the command once its delay is over. Typing takes a
// second or two, so the engine only queues the line; the companion types
// it and reports back with ChatLineTyped.
func (e *Engine) gameModeStep() {
	g := &e.host.gameMode
	if g.done || g.at.IsZero() || e.now().Before(g.at) {
		return
	}
	g.at, g.done = time.Time{}, true
	e.chatLines = append(e.chatLines, e.host.settings.GameMode)
}

// TakeChatLine returns the next line Game mode wants typed in the match
// chat, and false when there is none. Each line is returned once.
func (e *Engine) TakeChatLine() (string, bool) {
	if len(e.chatLines) == 0 {
		return "", false
	}
	line := e.chatLines[0]
	e.chatLines = e.chatLines[1:]
	return line, true
}

// ChatLineTyped reports how typing a line from TakeChatLine went: err is
// nil when it was typed, or an error whose text is a sentence for the
// player saying why it wasn't. Nothing is retried.
func (e *Engine) ChatLineTyped(line string, err error) {
	if err == nil {
		e.Note("Typed " + line + " in the game chat.")
		return
	}
	e.Note("Game mode didn't type " + line + ". " + err.Error())
}
