package follow_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func gameModeSettings(command string, delay int) dto.HostSettings {
	s := hostSettings()
	s.DelaySeconds = 0
	s.GameMode, s.GameModeDelaySeconds = command, delay
	return s
}

// startHostedMatch opens Auto-host's lobby and plays it into a match the
// way the game reported it live (issue 9): loading, the players getting
// ready, then DISABLED_SCREEN and IsGameUIActive true.
func (f *fixture) startHostedMatch() {
	f.openOwnLobby()
	f.receive("GameLobbyGracefulExit", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"})
	f.receive("LoadProgressUpdate", map[string]any{"fillPercentage": 1.0})
	f.receive("LoadingPlayerReadyUpdate", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "DISABLED_SCREEN"})
	f.receive("IsGameUIActive", map[string]any{"isActive": true})
}

// typed drains the engine's chat line requests.
func (f *fixture) typed() []string {
	var lines []string
	for {
		line, ok := f.engine.TakeChatLine()
		if !ok {
			return lines
		}
		lines = append(lines, line)
	}
}

func (f *fixture) waitAndTick(d time.Duration) {
	f.advance(d)
	f.engine.Tick()
}

func TestGameModeIsTypedOnceAfterTheDelay(t *testing.T) {
	f := newFixture(t)
	f.mustHost(gameModeSettings("  -ar  ", 5))
	f.startHostedMatch()

	f.waitAndTick(4 * time.Second)
	if got := f.typed(); len(got) != 0 {
		t.Fatalf("typed before the delay: %q", got)
	}
	f.waitAndTick(time.Second)
	if got := f.typed(); len(got) != 1 || got[0] != "-ar" {
		t.Fatalf("typed = %q, want [-ar]", got)
	}
	f.receive("IsGameUIActive", map[string]any{"isActive": true})
	f.waitAndTick(10 * time.Second)
	if got := f.typed(); len(got) != 0 {
		t.Fatalf("typed again in the same match: %q", got)
	}
}

func TestGameModeTypesNothingWhenTheCommandIsEmpty(t *testing.T) {
	f := newFixture(t)
	f.mustHost(gameModeSettings("   ", 0))
	f.startHostedMatch()
	f.waitAndTick(time.Minute)
	if got := f.typed(); len(got) != 0 {
		t.Fatalf("typed %q with Game mode off", got)
	}
}

func TestGameModeTypesNothingWhenTheMatchEndsFirst(t *testing.T) {
	for name, end := range map[string]func(f *fixture){
		"exited":       func(f *fixture) { f.receive("ExitedGame", nil) },
		"score screen": func(f *fixture) { f.receive("SetGlueScreen", map[string]any{"screen": "SCORE_SCREEN"}) },
		"left":         func(f *fixture) { f.receive("MultiplayerGameLeave", nil) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.mustHost(gameModeSettings("-ar", 5))
			f.startHostedMatch()
			f.waitAndTick(2 * time.Second)
			end(f)
			if got := f.state().Status; !strings.Contains(strings.Join(logMessages(f), "|"), "The match ended before Game mode typed -ar.") {
				t.Errorf("no cancel note; status = %q", got)
			}
			f.waitAndTick(time.Minute)
			if got := f.typed(); len(got) != 0 {
				t.Fatalf("typed %q after the match ended", got)
			}
		})
	}
}

func TestGameModeStillTypesWhenTheGameUIGoesInactive(t *testing.T) {
	// IsGameUIActive false may come from alt-tab or the in-game menu, so it
	// isn't the end of the match.
	f := newFixture(t)
	f.mustHost(gameModeSettings("-ar", 5))
	f.startHostedMatch()
	f.waitAndTick(2 * time.Second)
	f.receive("IsGameUIActive", map[string]any{"isActive": false})
	f.waitAndTick(3 * time.Second)
	if got := f.typed(); len(got) != 1 || got[0] != "-ar" {
		t.Fatalf("typed = %q, want [-ar]", got)
	}
}

func TestGameModeTypesNothingAfterADisconnect(t *testing.T) {
	f := newFixture(t)
	f.mustHost(gameModeSettings("-ar", 5))
	f.startHostedMatch()
	f.waitAndTick(2 * time.Second)
	f.engine.Connection(false, "")
	f.waitAndTick(time.Minute)
	if got := f.typed(); len(got) != 0 {
		t.Fatalf("typed %q after the game disconnected", got)
	}
}

func logMessages(f *fixture) []string {
	var out []string
	for _, entry := range f.state().Log {
		out = append(out, entry.Message)
	}
	return out
}

func TestGameModeTypesNothingInAMatchAutoHostDidNotCreate(t *testing.T) {
	f := newFixture(t)
	if err := f.engine.ConfigureHost(dto.HostConfig{Settings: ptr(gameModeSettings("-ar", 0))}); err != nil {
		t.Fatal(err)
	}
	// Someone else's lobby and match, with Auto-host off.
	f.receive("SetGlueScreen", map[string]any{"screen": "GAME_LOBBY"})
	f.receive("GameLobbySetup", lobbySetup(2, 2))
	f.receive("SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"})
	f.receive("SetGlueScreen", map[string]any{"screen": "DISABLED_SCREEN"})
	f.receive("IsGameUIActive", map[string]any{"isActive": true})
	f.waitAndTick(time.Minute)
	if got := f.typed(); len(got) != 0 {
		t.Fatalf("typed %q in a match Auto-host didn't create", got)
	}
}

func TestGameModeTypesNothingWhileAutoHostOnlyWaits(t *testing.T) {
	f := newFixture(t)
	s := gameModeSettings("-ar", 0)
	s.DelaySeconds = 30
	f.mustHost(s)
	// A match starts before Auto-host created its lobby.
	f.receive("SetGlueScreen", map[string]any{"screen": "DISABLED_SCREEN"})
	f.receive("IsGameUIActive", map[string]any{"isActive": true})
	f.waitAndTick(5 * time.Second)
	if got := f.typed(); len(got) != 0 {
		t.Fatalf("typed %q in a match Auto-host didn't create", got)
	}
}

func TestGameModeIsTypedAgainInTheNextHostedMatch(t *testing.T) {
	f := newFixture(t)
	f.mustHost(gameModeSettings("-ar", 0))
	f.startHostedMatch()
	if got := f.typed(); len(got) != 1 {
		t.Fatalf("first match typed %q", got)
	}
	f.receive("ExitedGame", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "SCORE_SCREEN"})
	f.receive("MultiplayerGameLeave", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "MAIN_MENU"})
	f.waitAndTick(3 * time.Second)
	f.startHostedMatch()
	if got := f.typed(); len(got) != 1 {
		t.Fatalf("second match typed %q", got)
	}
}

func TestGameModeDelayIsKeptWithinZeroAndTwoMinutes(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{-3, 0}, {0, 0}, {5, 5}, {120, 120}, {500, 120}} {
		s, err := follow.NormalizeHostSettings(gameModeSettings("-ar", tc.in))
		if err != nil || s.GameModeDelaySeconds != tc.want {
			t.Errorf("delay %d: got %d, %v; want %d", tc.in, s.GameModeDelaySeconds, err, tc.want)
		}
	}
}

func TestGameModeWaitsTheClampedDelay(t *testing.T) {
	f := newFixture(t)
	f.mustHost(gameModeSettings("-ar", 500))
	f.startHostedMatch()
	f.waitAndTick(119 * time.Second)
	if got := f.typed(); len(got) != 0 {
		t.Fatalf("typed early: %q", got)
	}
	f.waitAndTick(time.Second)
	if got := f.typed(); len(got) != 1 {
		t.Fatalf("not typed after 120 s: %q", got)
	}
}

func TestGameModeCommandMustBeOneChatLine(t *testing.T) {
	for _, bad := range []string{"-ar\n-sp", strings.Repeat("a", 101)} {
		if _, err := follow.NormalizeHostSettings(gameModeSettings(bad, 5)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	s, err := follow.NormalizeHostSettings(gameModeSettings("  -ar  ", 5))
	if err != nil || s.GameMode != "-ar" {
		t.Fatalf("got %q, %v", s.GameMode, err)
	}
}

func TestGameModeDelayDefaultsToFiveSeconds(t *testing.T) {
	if got := newFixture(t).engine.HostSettings().GameModeDelaySeconds; got != 5 {
		t.Fatalf("default delay = %d", got)
	}
}

func TestChatLineTypedReportsTheResult(t *testing.T) {
	f := newFixture(t)
	f.engine.ChatLineTyped("-ar", nil)
	if got := f.state().Status; got != "Typed -ar in the game chat." {
		t.Fatalf("status = %q", got)
	}
	f.engine.ChatLineTyped("-ar", errors.New("Warcraft III wasn't in front."))
	if got := f.state().Status; got != "Game mode didn't type -ar. Warcraft III wasn't in front." {
		t.Fatalf("status = %q", got)
	}
}
