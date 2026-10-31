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
// player seated next to a stranger (a player the player doesn't follow).
func guestLobby(name string, isHost bool) map[string]any {
	return seatedLobby(name, isHost, "Stranger#5678")
}

// seatedLobby is a GameLobbySetup with the player seated and the named human
// players in the other slots.
func seatedLobby(name string, isHost bool, others ...string) map[string]any {
	players := []any{map[string]any{"isSelf": true, "slotStatus": 2, "slotType": 0, "name": "Self#1111"}}
	for _, o := range others {
		players = append(players, map[string]any{"slotStatus": 2, "slotType": 0, "name": o})
	}
	return map[string]any{"lobbyName": name, "isHost": isHost, "playerHost": "Friend#1234", "players": players}
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

func TestGreetsAsHostOnceAStrangerJoins(t *testing.T) {
	f := greetFixture(t)
	f.receive("GameLobbySetup", seatedLobby("My game", true))
	f.wait(12 * time.Second)
	if got := f.chat(); len(got) != 0 {
		t.Fatalf("greeted an empty lobby: %q", got)
	}
	f.receive("GameLobbySetup", seatedLobby("My game", true, "Stranger#5678"))
	f.wait(6 * time.Second)
	if got := f.chat(); len(got) != 0 {
		t.Fatalf("greeted before the delay: %q", got)
	}
	f.wait(time.Second)
	if got := f.chat(); !slices.Equal(got, []string{"hey"}) {
		t.Fatalf("chat = %q, want one greeting 6.5 s after the stranger joined", got)
	}
}

func TestGreetsInAnAutoHostLobbyOnceAStrangerJoins(t *testing.T) {
	f := greetFixture(t)
	f.mustHost(hostSettings())
	f.receive("GameLobbySetup", seatedLobby("My game", true, "Stranger#5678"))
	f.wait(7 * time.Second)
	if got := f.chat(); !slices.Contains(got, "hey") {
		t.Fatalf("chat = %q, want the greeting in Auto-host's lobby", got)
	}
}

func TestDoesNotGreetOnlyThePlayersIFollowOrComputers(t *testing.T) {
	f := greetFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Friend#1234"}}); err != nil {
		t.Fatal(err)
	}
	lobby := seatedLobby("Fun game", false, "friend#1234")
	players := lobby["players"].([]any)
	lobby["players"] = append(players, map[string]any{"slotStatus": 2, "slotType": 1, "name": "Computer (Normal)"},
		map[string]any{"slotStatus": 0, "slotType": 0, "name": ""})
	f.receive("GameLobbySetup", lobby)
	f.wantNoChat()
}

func TestGreetsWhenTheFirstStrangerJoinsAfterMyFriends(t *testing.T) {
	f := greetFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Friend#1234"}}); err != nil {
		t.Fatal(err)
	}
	f.receive("GameLobbySetup", seatedLobby("Fun game", false, "Friend#1234"))
	f.wait(12 * time.Second)
	f.receive("GameLobbySetup", seatedLobby("Fun game", false, "Friend#1234", "Stranger#5678"))
	f.receive("GameLobbySetup", seatedLobby("Fun game", false, "Friend#1234", "Stranger#5678", "Other#9012"))
	f.wait(20 * time.Second)
	if got := f.chat(); len(got) != 1 {
		t.Fatalf("chat = %q, want exactly one greeting after the first stranger", got)
	}
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

func lobbyChat(sender, content string) map[string]any {
	return map[string]any{"message": map[string]any{"source": "gameChat", "type": "message", "sender": sender, "content": content}}
}

// Someone else greets first: the greeting follows 2 to 4 seconds after it
// (3 s with the fixed draw) instead of its own 6.5 s.
func TestGreetingAnswersAnotherPlayersGreeting(t *testing.T) {
	f := greetFixture(t)
	f.receive("GameLobbySetup", guestLobby("Fun game", false))
	f.advance(time.Second)
	f.receive("ChatMessage", lobbyChat("Stranger#5555", "Hey all!"))
	f.advance(2900 * time.Millisecond)
	f.engine.Tick()
	if got := f.chat(); len(got) != 0 {
		t.Fatalf("typed %q too early", got)
	}
	f.advance(200 * time.Millisecond)
	f.engine.Tick()
	if got := f.chat(); len(got) != 1 {
		t.Fatalf("chat = %q, want one greeting 3 s after theirs", got)
	}
	f.wait(10 * time.Second)
	if got := f.chat(); len(got) != 1 {
		t.Fatalf("chat = %q, want still one", got)
	}
}

func TestGreetingReplyDelayStaysBetweenTwoAndFourSeconds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		draw  float64
		first time.Duration
	}{{"shortest", 0, 2 * time.Second}, {"longest", 0.999, 4 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.engine.SetRandom(edgeRandom{tc.draw})
			f.receive("GameLobbySetup", guestLobby("Fun game", false))
			f.receive("ChatMessage", lobbyChat("Stranger#5555", "hi"))
			f.advance(tc.first - 50*time.Millisecond)
			f.engine.Tick()
			if tc.draw == 0 && len(f.chat()) != 0 {
				t.Fatal("typed before two seconds")
			}
			f.advance(100 * time.Millisecond)
			f.engine.Tick()
			if got := f.chat(); len(got) != 1 {
				t.Fatalf("chat = %q after %s", got, tc.first)
			}
		})
	}
}

// Lines that aren't greetings, other chats and info lines keep the normal
// delay; only the first greeting moves it.
func TestGreetingIgnoresOtherChat(t *testing.T) {
	f := greetFixture(t)
	f.receive("GameLobbySetup", guestLobby("Fun game", false))
	f.receive("ChatMessage", lobbyChat("Stranger#5555", "this map is great"))
	f.receive("ChatMessage", lobbyChat("Stranger#5555", "highground wins"))
	f.receive("ChatMessage", map[string]any{"message": map[string]any{"source": "info", "content": "Hi joined the game."}})
	f.receive("ChatMessage", map[string]any{"message": map[string]any{"source": "whisper", "sender": "Friend#1234", "content": "hi"}})
	f.advance(6400 * time.Millisecond)
	f.engine.Tick()
	if got := f.chat(); len(got) != 0 {
		t.Fatalf("typed %q too early", got)
	}
	f.advance(200 * time.Millisecond)
	f.engine.Tick()
	if got := f.chat(); len(got) != 1 {
		t.Fatalf("chat = %q, want the greeting at its own time", got)
	}
}

func TestIsGreeting(t *testing.T) {
	for line, want := range map[string]bool{
		"hi": true, "Hey all!": true, "HELLO there": true, "glhf": true, "yo": true, "  hiya :)": true,
		"highground": false, "this is hi": false, "": false, "!swap 1 2": false, "gg": false,
	} {
		if got := follow.IsGreeting(line); got != want {
			t.Errorf("IsGreeting(%q) = %v, want %v", line, got, want)
		}
	}
}
