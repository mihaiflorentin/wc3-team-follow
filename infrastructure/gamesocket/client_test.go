package gamesocket_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/gamesocket"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestDecodeKeepsAccountIDPrecision(t *testing.T) {
	ev := gamesocket.Decode([]byte(`{"messageType":"FriendsFriendUpdated","payload":{"data":{"friend":{"accountId":9007199254740993}}}}`))
	id := ev.Payload["data"].(map[string]any)["friend"].(map[string]any)["accountId"]
	if id != json.Number("9007199254740993") {
		t.Fatalf("accountId = %#v", id)
	}
}

func TestDecodeFlagsMalformedMessages(t *testing.T) {
	for _, body := range []string{"nope", "null", `{"payload":{}}`, `{"messageType":7}`} {
		if !gamesocket.Decode([]byte(body)).Malformed {
			t.Errorf("%s not flagged", body)
		}
	}
}

func TestSessionSendsAllowedCommandsAndRejectsOthers(t *testing.T) {
	received := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webui-socket/12345678901" {
			http.NotFound(w, r)
			return
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		_, body, err := ws.Read(r.Context())
		if err != nil {
			return
		}
		received <- string(body)
		_ = ws.Write(r.Context(), websocket.MessageText, []byte(`{"messageType":"TeamsLeft","payload":{}}`))
		_, _, _ = ws.Read(r.Context())
	}))
	defer server.Close()
	port, _ := strconv.Atoi(server.URL[strings.LastIndex(server.URL, ":")+1:])

	ctx := context.Background()
	session, err := gamesocket.NewDialer(slog.New(slog.DiscardHandler)).Dial(ctx, dto.GameConnection{Port: port, Session: "12345678901"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	if err := session.Send(ctx, "GetGameInfo", nil); !errors.Is(err, gamesocket.ErrNotAllowed) {
		t.Fatalf("GetGameInfo: err = %v", err)
	}
	if err := session.Send(ctx, "TeamGetInfo", nil); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != `{"type":"webui","message":"TeamGetInfo","payload":{}}` {
		t.Fatalf("sent %s", got)
	}
	ev, err := session.Receive(ctx)
	if err != nil || ev.MessageType != "TeamsLeft" {
		t.Fatalf("received %+v, %v", ev, err)
	}
}

func TestSessionSendsTheLobbyChatMessage(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		_, body, err := ws.Read(r.Context())
		if err != nil {
			return
		}
		received <- string(body)
	}))
	defer server.Close()
	port, _ := strconv.Atoi(server.URL[strings.LastIndex(server.URL, ":")+1:])

	ctx := context.Background()
	session, err := gamesocket.NewDialer(slog.New(slog.DiscardHandler)).Dial(ctx, dto.GameConnection{Port: port, Session: "12345678901"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	if err := session.Send(ctx, "SendGameChatMessage", map[string]any{"content": "!swap 1 2"}); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != `{"type":"webui","message":"SendGameChatMessage","payload":{"content":"!swap 1 2"}}` {
		t.Fatalf("sent %s", got)
	}
}

// TestSessionSendsTheLobbyBanButNotKick covers auto-ban (docs/adr/0008):
// the game's own Ban is allowed, Kick never was.
func TestSessionSendsTheLobbyBanButNotKick(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		_, body, err := ws.Read(r.Context())
		if err != nil {
			return
		}
		received <- string(body)
	}))
	defer server.Close()
	port, _ := strconv.Atoi(server.URL[strings.LastIndex(server.URL, ":")+1:])

	ctx := context.Background()
	session, err := gamesocket.NewDialer(slog.New(slog.DiscardHandler)).Dial(ctx, dto.GameConnection{Port: port, Session: "12345678901"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	if err := session.Send(ctx, "KickPlayerFromGameLobby", map[string]any{"slot": 2}); !errors.Is(err, gamesocket.ErrNotAllowed) {
		t.Fatalf("KickPlayerFromGameLobby: err = %v", err)
	}
	if err := session.Send(ctx, "BanPlayerFromGameLobby", map[string]any{"slot": json.Number("2")}); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != `{"type":"webui","message":"BanPlayerFromGameLobby","payload":{"slot":2}}` {
		t.Fatalf("sent %s", got)
	}
}

func TestTryReceiveReturnsWaitingEventsWithoutBlocking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		_ = ws.Write(r.Context(), websocket.MessageText, []byte(`{"messageType":"GameLobbySetup","payload":{}}`))
		_ = ws.Write(r.Context(), websocket.MessageText, []byte(`{"messageType":"ChatMessage","payload":{}}`))
		_, _, _ = ws.Read(r.Context())
	}))
	defer server.Close()
	port, _ := strconv.Atoi(server.URL[strings.LastIndex(server.URL, ":")+1:])

	ctx := context.Background()
	session, err := gamesocket.NewDialer(slog.New(slog.DiscardHandler)).Dial(ctx, dto.GameConnection{Port: port, Session: "12345678901"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := session.Receive(ctx)
	if err != nil || first.MessageType != "GameLobbySetup" {
		t.Fatalf("Receive = %+v, %v", first, err)
	}
	var second dto.GameEvent
	for deadline := time.Now().Add(3 * time.Second); second.MessageType == ""; {
		ev, ok, err := session.TryReceive()
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("TryReceive: %v", err)
		}
		if ok {
			second = ev
		}
	}
	if second.MessageType != "ChatMessage" {
		t.Fatalf("second = %+v", second)
	}
	if _, ok, err := session.TryReceive(); ok || err != nil {
		t.Fatalf("nothing waiting: ok = %v, err = %v", ok, err)
	}
	_ = session.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, _, err := session.TryReceive(); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("TryReceive reported no error after Close")
		}
	}
}

func TestCloseIsPromptWithAFullEventQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		// More than the session queues, so its reader waits to hand one on.
		for range 400 {
			if ws.Write(r.Context(), websocket.MessageText, []byte(`{"messageType":"ChatMessage","payload":{}}`)) != nil {
				return
			}
		}
		_, _, _ = ws.Read(r.Context())
	}))
	defer server.Close()
	port, _ := strconv.Atoi(server.URL[strings.LastIndex(server.URL, ":")+1:])

	ctx := context.Background()
	session, err := gamesocket.NewDialer(slog.New(slog.DiscardHandler)).Dial(ctx, dto.GameConnection{Port: port, Session: "12345678901"})
	if err != nil {
		t.Fatal(err)
	}
	// Wait until the queue is full.
	if _, err := session.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	done := make(chan struct{})
	start := time.Now()
	go func() {
		_ = session.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close never returned")
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("Close took %v with a full queue", took)
	}
	// The queued events drain, then the end of the session is reported.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, _, err := session.TryReceive(); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("TryReceive reported no error after Close")
		}
	}
}

func TestDialRejectsPrivilegedPorts(t *testing.T) {
	if _, err := gamesocket.NewDialer(slog.New(slog.DiscardHandler)).Dial(context.Background(), dto.GameConnection{Port: 80}); err == nil {
		t.Fatal("dialled port 80")
	}
}
