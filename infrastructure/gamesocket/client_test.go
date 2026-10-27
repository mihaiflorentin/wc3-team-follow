package gamesocket_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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
	session, err := gamesocket.NewDialer().Dial(ctx, dto.GameConnection{Port: port, Session: "12345678901"})
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
	session, err := gamesocket.NewDialer().Dial(ctx, dto.GameConnection{Port: port, Session: "12345678901"})
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

func TestDialRejectsPrivilegedPorts(t *testing.T) {
	if _, err := gamesocket.NewDialer().Dial(context.Background(), dto.GameConnection{Port: 80}); err == nil {
		t.Fatal("dialled port 80")
	}
}
