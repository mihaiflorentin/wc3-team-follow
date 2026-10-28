// Package gamesocket talks to the game's local menu service over its
// loopback websocket.
package gamesocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/coder/websocket"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	dialTimeout = 5 * time.Second
	readLimit   = 8 << 20
	// queued is how many received events wait for Receive or TryReceive
	// before the reader waits too.
	queued = 256
)

// allowed lists every command the companion may send, each one the game's
// own menu sends for the same purpose. Two state queries (SendGameLobbySetup,
// GetGameInfo) closed the game during development and must never be added.
var allowed = map[string]bool{
	// Watching and Auto-join.
	"TeamGetInfo":                  true,
	"FriendsGetFriends":            true,
	"RequestCancelTeamInvitations": true,
	"TeamLeave":                    true,
	"JoinFriendGameLobby":          true,
	"JoinGameByGameName":           true,
	// The Custom Games screen's listing requests: joining by name only finds
	// lobbies in the list they load.
	"SendGameListing": true,
	"GetGameList":     true,
	// Auto-host: Create Game's create button, and the host's lobby controls.
	"CreateLobby": true,
	"LobbyStart":  true,
	"LobbyCancel": true,
	"SetTeam":     true,
	// Auto-invite: the friends list's Invite to game.
	"SendInviteToGame": true,
	// The lobby and match chat box (docs/adr/0006). Only Auto-host's own
	// commands may be typed, one short line at a time (domain/follow).
	"SendGameChatMessage": true,
	// Auto-ban: the Ban item of a lobby slot's name menu (docs/adr/0008).
	// Only for blacklisted players in a lobby the player hosts
	// (domain/follow). KickPlayerFromGameLobby is not allowed.
	"BanPlayerFromGameLobby": true,
}

// ErrNotAllowed reports a command outside the allowlist.
var ErrNotAllowed = errors.New("unsupported game command")

// Dialer opens sessions on 127.0.0.1 only.
type Dialer struct {
	logger *slog.Logger
}

// NewDialer returns the game socket dialer. logger records a panic in a
// session's reader goroutine.
func NewDialer(logger *slog.Logger) contract.GameDialer { return Dialer{logger: logger} }

// Dial connects to ws://127.0.0.1:<port>/webui-socket/<session>.
func (d Dialer) Dial(ctx context.Context, conn dto.GameConnection) (contract.GameSession, error) {
	if conn.Port < 1024 || conn.Port > 65535 {
		return nil, fmt.Errorf("dial game: invalid port %d", conn.Port)
	}
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	url := fmt.Sprintf("ws://127.0.0.1:%d/webui-socket/%s", conn.Port, conn.Session)
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		// The URL holds the session: never include it in the error.
		return nil, errors.New("dial game: the menu service refused the connection")
	}
	ws.SetReadLimit(readLimit)
	readCtx, stop := context.WithCancel(context.Background())
	s := &Session{ws: ws, events: make(chan dto.GameEvent, queued), stop: stop, logger: d.logger}
	go s.readAll(readCtx)
	return s, nil
}

// Session is one open connection to the menu service. A reader goroutine
// receives events as they arrive, so TryReceive can return the ones
// already waiting.
type Session struct {
	ws     *websocket.Conn
	events chan dto.GameEvent
	stop   context.CancelFunc
	logger *slog.Logger
	// err is why reading ended; set before events is closed.
	err error
}

// readAll reads events until the connection ends, then closes events.
func (s *Session) readAll(ctx context.Context) {
	defer s.guard()
	defer close(s.events)
	for {
		_, body, err := s.ws.Read(ctx)
		if err != nil {
			s.err = fmt.Errorf("receive: %w", err)
			return
		}
		select {
		case s.events <- Decode(body):
		case <-ctx.Done():
			s.err = fmt.Errorf("receive: %w", ctx.Err())
			return
		}
	}
}

// guard logs a panic in the reader goroutine with its stack, then lets it
// crash the app as it would have anyway, so the log explains the crash.
// Event payloads are never logged.
func (s *Session) guard() {
	if r := recover(); r != nil {
		s.logger.Error("gamesocket.panic", slog.String("where", "read"), slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
		panic(r)
	}
}

type outbound struct {
	Type    string         `json:"type"`
	Message string         `json:"message"`
	Payload map[string]any `json:"payload"`
}

// Send writes one allow-listed command.
func (s *Session) Send(ctx context.Context, message string, payload map[string]any) error {
	if !allowed[message] {
		return fmt.Errorf("%w: %s", ErrNotAllowed, message)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(outbound{Type: "webui", Message: message, Payload: payload})
	if err != nil {
		return fmt.Errorf("encode %s: %w", message, err)
	}
	if err := s.ws.Write(ctx, websocket.MessageText, body); err != nil {
		return fmt.Errorf("send %s: %w", message, err)
	}
	return nil
}

// Receive returns the next event. A message that is not a JSON object with
// a string messageType comes back with Malformed set.
func (s *Session) Receive(ctx context.Context) (dto.GameEvent, error) {
	select {
	case ev, ok := <-s.events:
		if !ok {
			return dto.GameEvent{}, s.err
		}
		return ev, nil
	case <-ctx.Done():
		return dto.GameEvent{}, fmt.Errorf("receive: %w", ctx.Err())
	}
}

// TryReceive returns the next event already received, without waiting.
func (s *Session) TryReceive() (dto.GameEvent, bool, error) {
	select {
	case ev, ok := <-s.events:
		if !ok {
			return dto.GameEvent{}, false, s.err
		}
		return ev, true, nil
	default:
		return dto.GameEvent{}, false, nil
	}
}

// Decode parses one event. Numbers stay json.Number so account
// identifiers keep their precision.
func Decode(body []byte) dto.GameEvent {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil || raw == nil {
		return dto.GameEvent{Malformed: true}
	}
	kind, ok := raw["messageType"].(string)
	if !ok {
		return dto.GameEvent{Malformed: true}
	}
	payload, _ := raw["payload"].(map[string]any)
	return dto.GameEvent{MessageType: kind, Payload: payload}
}

// Close ends the session. The reader is stopped first, so it never waits
// on a full event queue while the connection closes.
func (s *Session) Close() error {
	s.stop()
	return s.ws.Close(websocket.StatusNormalClosure, "")
}
