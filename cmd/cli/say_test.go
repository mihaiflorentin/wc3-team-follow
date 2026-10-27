package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	mockgame "github.com/mihaiflorentin/wc3-team-follow/mock/game"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func runSay(t *testing.T, d *mockgame.Discovery, s *mockgame.Session, args ...string) (string, error) {
	t.Helper()
	cmd := sayCmd(func() (contract.GameDiscovery, error) { return d, nil },
		func() (contract.GameDialer, error) { return &mockgame.Dialer{Session: s}, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append(args, "--wait", "50ms"))
	err := cmd.Execute()
	return out.String(), err
}

func TestSaySendsOneTrimmedLineAndPrintsChatEvents(t *testing.T) {
	d := &mockgame.Discovery{Conn: dto.GameConnection{PID: 7, Session: "12345678901234", Version: "3.0.1.24371"}}
	s := mockgame.NewSession()
	s.Push(dto.GameEvent{MessageType: "ChatMessage", Payload: map[string]any{"message": map[string]any{"source": "gameChat", "type": "message", "content": "-ar"}}})
	s.Push(dto.GameEvent{MessageType: "FriendsFriendData", Payload: map[string]any{"friends": []any{map[string]any{"battleTag": "Secret#1", "auroraId": 99}}}})
	out, err := runSay(t, d, s, "  -ar  ")
	if err != nil {
		t.Fatal(err)
	}
	sent := s.Sent()
	if len(sent) != 1 || sent[0].Message != "SendGameChatMessage" || sent[0].Payload["content"] != "-ar" {
		t.Fatalf("sent = %+v", sent)
	}
	if !strings.Contains(out, `content="-ar"`) || !strings.Contains(out, "FriendsFriendData") {
		t.Fatalf("output:\n%s", out)
	}
	for _, secret := range []string{"12345678901234", "Secret#1", "99"} {
		if strings.Contains(out, secret) {
			t.Fatalf("printed %q:\n%s", secret, out)
		}
	}
}

func TestSayRejectsBadTextWithoutConnecting(t *testing.T) {
	d := &mockgame.Discovery{}
	s := mockgame.NewSession()
	for _, text := range []string{"   ", "two\nlines", strings.Repeat("a", 101)} {
		if _, err := runSay(t, d, s, text); err == nil {
			t.Errorf("%q: no error", text)
		}
	}
	if d.Calls != 0 || len(s.Sent()) != 0 {
		t.Fatalf("discovered %d times, sent %d", d.Calls, len(s.Sent()))
	}
}

func TestSayReportsWhenTheGameIsNotFound(t *testing.T) {
	d := &mockgame.Discovery{Err: mockgame.CountedError{Message: "Warcraft III is running 2 times.", Count: 2}}
	_, err := runSay(t, d, mockgame.NewSession(), "hi")
	if err == nil || !strings.Contains(err.Error(), "running 2 times") {
		t.Fatalf("err = %v", err)
	}
	d = &mockgame.Discovery{Err: errors.New("Warcraft III isn't running.")}
	if _, err := runSay(t, d, mockgame.NewSession(), "hi"); err == nil || !strings.Contains(err.Error(), "isn't running") {
		t.Fatalf("err = %v", err)
	}
}
