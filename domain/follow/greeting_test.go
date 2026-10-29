package follow_test

import (
	"slices"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// fixedRandom picks index pick and a delay of 6.5 s (3 s plus half of the
// 7 s range).
type fixedRandom struct{ pick int }

func (r fixedRandom) IntN(int) int     { return r.pick }
func (r fixedRandom) Float64() float64 { return 0.5 }

func greetFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.engine.SetRandom(fixedRandom{pick: 1})
	return f
}

// guestLobby is a GameLobbySetup for a lobby someone else hosts, with the
// player seated.
func guestLobby(name string, isHost bool) map[string]any {
	return map[string]any{"lobbyName": name, "isHost": isHost, "playerHost": "Friend#1234",
		"players": []any{map[string]any{"isSelf": true, "slotStatus": 2}}}
}

func (f *fixture) chat() []string {
	var lines []string
	for _, c := range f.commands {
		if c.message == follow.CmdSendGameChatMessage {
			lines = append(lines, c.payload["content"].(string))
		}
	}
	return lines
}

// wait lets d pass in one-second ticks, as the companion ticks.
func (f *fixture) wait(d time.Duration) {
	for ; d > 0; d -= time.Second {
		f.advance(min(d, time.Second))
		f.engine.Tick()
	}
}

func (f *fixture) logged(message string) bool {
	return slices.ContainsFunc(f.state().Log, func(e dto.LogEntry) bool { return e.Message == message })
}

func (f *fixture) wantNoChat() {
	f.t.Helper()
	f.wait(10 * time.Second)
	if got := f.chat(); len(got) != 0 {
		f.t.Fatalf("typed %q, want nothing", got)
	}
}

func TestGreetsOnceAfterTheDelayWhenJoiningAsGuest(t *testing.T) {
	f := greetFixture(t)
	f.receive("GameLobbySetup", guestLobby("Fun game", false))
	f.advance(6400 * time.Millisecond)
	f.engine.Tick()
	if got := f.chat(); len(got) != 0 {
		t.Fatalf("typed before the delay: %q", got)
	}
	f.advance(200 * time.Millisecond)
	f.engine.Tick()
	if got := f.chat(); len(got) != 1 || got[0] != "hey" {
		t.Fatalf("chat = %q, want one hey", got)
	}
	if !slices.Contains(follow.Greetings(), "hey") {
		t.Fatal("hey is not in the list")
	}
	if !f.logged("Said “hey” in the lobby.") {
		t.Fatalf("log = %+v", f.state().Log)
	}
}

func TestGreetingDelayStaysBetweenThreeAndTenSeconds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		draw  float64
		first time.Duration
	}{{"shortest", 0, 3 * time.Second}, {"longest", 0.999, 10 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.engine.SetRandom(edgeRandom{tc.draw})
			f.receive("GameLobbySetup", guestLobby("Fun game", false))
			f.advance(tc.first - 50*time.Millisecond)
			f.engine.Tick()
			if tc.draw == 0 && len(f.chat()) != 0 {
				t.Fatal("typed before three seconds")
			}
			f.advance(100 * time.Millisecond)
			f.engine.Tick()
			if len(f.chat()) != 1 {
				t.Fatalf("chat = %q after %v", f.chat(), tc.first)
			}
		})
	}
}

type edgeRandom struct{ draw float64 }

func (r edgeRandom) IntN(int) int     { return 0 }
func (r edgeRandom) Float64() float64 { return r.draw }

func TestEveryGreetingIsAValidChatLine(t *testing.T) {
	want := []string{"hi", "hey", "hello", "hi all", "hey everyone", "hello there", "hiya", "glhf"}
	if got := follow.Greetings(); !slices.Equal(got, want) {
		t.Fatalf("greetings = %q", got)
	}
	for _, g := range want {
		if _, err := follow.ChatLine(g); err != nil {
			t.Errorf("%q: %v", g, err)
		}
	}
}

func TestNoGreetingStartsWithACapitalLetter(t *testing.T) {
	for _, g := range follow.Greetings() {
		first, _ := utf8.DecodeRuneInString(g)
		if !unicode.IsLower(first) {
			t.Errorf("%q does not start with a lowercase letter", g)
		}
	}
}

func TestDoesNotGreetAsHost(t *testing.T) {
	f := greetFixture(t)
	f.receive("GameLobbySetup", guestLobby("My game", true))
	f.wantNoChat()
}

func TestDoesNotGreetWhenIsHostIsUnknown(t *testing.T) {
	f := greetFixture(t)
	p := guestLobby("Fun game", false)
	delete(p, "isHost")
	f.receive("GameLobbySetup", p)
	f.wantNoChat()
}

func TestDoesNotGreetInAutoHostLobby(t *testing.T) {
	f := greetFixture(t)
	f.mustHost(hostSettings())
	f.receive("GameLobbySetup", guestLobby("My game", false))
	f.wantNoChat()
}

func TestDoesNotGreetWhenNotSeated(t *testing.T) {
	f := greetFixture(t)
	p := guestLobby("Fun game", false)
	p["players"] = []any{map[string]any{"isSelf": true, "slotStatus": 0}}
	f.receive("GameLobbySetup", p)
	f.wantNoChat()
}

func TestGreetsOncePerLobby(t *testing.T) {
	f := greetFixture(t)
	f.receive("GameLobbySetup", guestLobby("Fun game", false))
	f.wait(7 * time.Second)
	f.receive("GameLobbySetup", guestLobby("Fun game", false))
	f.wait(10 * time.Second)
	if got := f.chat(); len(got) != 1 {
		t.Fatalf("chat = %q, want one greeting", got)
	}
}

func TestGreetsAgainInANewLobby(t *testing.T) {
	f := greetFixture(t)
	f.receive("GameLobbySetup", guestLobby("First", false))
	f.wait(7 * time.Second)
	f.receive("MultiplayerGameLeave", nil)
	f.receive("GameLobbySetup", guestLobby("First", false))
	f.wait(7 * time.Second)
	f.receive("GameLobbySetup", guestLobby("Second", false))
	f.wait(7 * time.Second)
	if got := f.chat(); len(got) != 3 {
		t.Fatalf("chat = %q, want three greetings", got)
	}
}

func TestGreetingIsCancelled(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event string
		p     map[string]any
	}{
		{"leaving the lobby", "MultiplayerGameLeave", nil},
		{"the countdown", "LobbyCountdown", map[string]any{"details": map[string]any{"countdownActive": true}}},
		{"a match loading", "GameFound", nil},
		{"the match screen", "IsGameUIActive", map[string]any{"isActive": true}},
		{"a disconnect", "LoggedOut", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := greetFixture(t)
			f.receive("GameLobbySetup", guestLobby("Fun game", false))
			f.wait(time.Second)
			f.receive(tc.event, tc.p)
			f.wantNoChat()
		})
	}
}

func TestDoesNotGreetDuringAMatch(t *testing.T) {
	f := greetFixture(t)
	f.receive("IsGameUIActive", map[string]any{"isActive": true})
	f.receive("GameLobbySetup", guestLobby("Fun game", false))
	f.wantNoChat()
}

func TestDoesNotTypeWhenTheConnectionIsClosedAndDoesNotRetry(t *testing.T) {
	f := greetFixture(t)
	f.fail = follow.CmdSendGameChatMessage
	f.receive("GameLobbySetup", guestLobby("Fun game", false))
	f.wait(10 * time.Second)
	if n := f.count(follow.CmdSendGameChatMessage); n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
}
