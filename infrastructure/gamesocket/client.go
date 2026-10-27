// Package gamesocket talks to the game's local menu service over its
// loopback websocket.
package gamesocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/coder/websocket"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	dialTimeout = 5 * time.Second
	readLimit   = 8 << 20
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
	// The Custom Games screen's listing request: joining by name only finds
	// lobbies in the list it loads.
	"GetGameList": true,
	// Auto-host: Create Game's create button, and the host's lobby controls.
	"CreateLobby": true,
	"LobbyStart":  true,
	"LobbyCancel": true,
	"SetTeam":     true,
	// Auto-invite: the friends list's Invite to game.
	"SendInviteToGame": true,
}

// ErrNotAllowed reports a command outside the allowlist.
var ErrNotAllowed = errors.New("unsupported game command")

// Dialer opens sessions on 127.0.0.1 only.
type Dialer struct{}

// NewDialer returns the game socket dialer.
func NewDialer() contract.GameDialer { return Dialer{} }

// Dial connects to ws://127.0.0.1:<port>/webui-socket/<session>.
func (Dialer) Dial(ctx context.Context, conn dto.GameConnection) (contract.GameSession, error) {
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
	return &Session{ws: ws}, nil
}

// Session is one open connection to the menu service.
type Session struct {
	ws *websocket.Conn
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
	_, body, err := s.ws.Read(ctx)
	if err != nil {
		return dto.GameEvent{}, fmt.Errorf("receive: %w", err)
	}
	return Decode(body), nil
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

// Close ends the session.
func (s *Session) Close() error {
	return s.ws.Close(websocket.StatusNormalClosure, "")
}
