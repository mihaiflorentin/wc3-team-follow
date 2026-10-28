package companion_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// playHostedMatch turns Auto-host on with Game mode and plays its lobby
// into a match. The service must be running.
func (h *harness) playHostedMatch(t *testing.T, command string, delay int) {
	t.Helper()
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	settings := dto.HostSettings{MapPath: "/detected/Maps/Legion TD.w3x", MapName: "Legion TD", GameName: "LTD", GameMode: command, GameModeDelaySeconds: delay}
	on := true
	if _, err := h.svc.ConfigureHost(context.Background(), dto.HostConfig{Settings: &settings, Enabled: &on, FromMenus: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "CreateLobby", func() bool { return slices.Contains(sentMessages(h.session), "CreateLobby") })
	for _, ev := range []dto.GameEvent{
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "GAME_LOBBY"}},
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "LOADING_SCREEN"}},
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "DISABLED_SCREEN"}},
		{MessageType: "IsGameUIActive", Payload: map[string]any{"isActive": true}},
	} {
		h.session.Push(ev)
	}
}

func TestGameModeTypesTheCommandInTheGame(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.discovery.Conn.PID = 4242
	h.run(t)
	h.playHostedMatch(t, "-ar", 0)
	waitFor(t, "the typed note", func() bool { return h.svc.State().Status == "Typed -ar in the game chat." })
	if got := h.keyboard.Typed(); !slices.Equal(got, []string{"-ar"}) || !slices.Equal(h.keyboard.PIDs, []int{4242}) {
		t.Fatalf("typed %q into %v", got, h.keyboard.PIDs)
	}
}

func TestGameModeSaysWhyTypingFailed(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.keyboard.Err = errors.New("Warcraft III wasn't in front, so typing stopped.")
	h.run(t)
	h.playHostedMatch(t, "-ar", 0)
	want := "Game mode didn't type -ar. Warcraft III wasn't in front, so typing stopped."
	waitFor(t, "the failure note", func() bool { return h.svc.State().Status == want })
	if got := h.keyboard.Typed(); len(got) != 1 {
		t.Fatalf("typed %q; it must not retry", got)
	}
}

func TestGameModeDoesNotTypeWithoutPermission(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.keyboard.Current = dto.KeyboardStatus{Supported: true, Reason: "Allow Team Follow to type in Accessibility settings."}
	h.run(t)
	h.playHostedMatch(t, "-ar", 0)
	want := "Game mode didn't type -ar. Allow Team Follow to type in Accessibility settings."
	waitFor(t, "the failure note", func() bool { return h.svc.State().Status == want })
	if got := h.keyboard.Typed(); len(got) != 0 {
		t.Fatalf("typed %q without permission", got)
	}
}

func TestKeyboardStatusIsInTheStateAndPermissionIsAskedOnRequest(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.keyboard.Current = dto.KeyboardStatus{Supported: true, Reason: "Allow Team Follow to type in Accessibility settings."}
	h.run(t)
	waitFor(t, "keyboard status", func() bool { return h.svc.State().Keyboard == h.keyboard.Current })
	if h.keyboard.Requests != 0 {
		t.Fatal("asked for the permission without the player")
	}
	state, err := h.svc.RequestKeyboardPermission(context.Background())
	if err != nil || h.keyboard.Requests != 1 || state.Keyboard != h.keyboard.Current {
		t.Fatalf("state=%+v err=%v requests=%d", state.Keyboard, err, h.keyboard.Requests)
	}
}

func TestRunWaitsForTypingToFinish(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.keyboard.Hold = make(chan struct{})
	stop := h.start()
	h.playHostedMatch(t, "-ar", 0)
	waitFor(t, "typing", func() bool { return len(h.keyboard.Typed()) == 1 })
	done := stop()
	select {
	case <-done:
		t.Fatal("Run returned while a line was still being typed")
	case <-time.After(100 * time.Millisecond):
	}
	close(h.keyboard.Hold)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run didn't return once typing finished")
	}
}

func TestTypingKeepsTheNewerKeyboardStatus(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.keyboard.Hold = make(chan struct{})
	h.run(t)
	h.playHostedMatch(t, "-ar", 0)
	waitFor(t, "typing", func() bool { return len(h.keyboard.Typed()) == 1 })
	revoked := dto.KeyboardStatus{Supported: true, Reason: "Allow Team Follow to type in Accessibility settings."}
	h.keyboard.SetStatus(revoked)
	waitFor(t, "the newer status", func() bool { return h.svc.State().Keyboard == revoked })
	close(h.keyboard.Hold)
	waitFor(t, "the typed note", func() bool { return h.svc.State().Status == "Typed -ar in the game chat." })
	if got := h.svc.State().Keyboard; got != revoked {
		t.Fatalf("keyboard status = %+v, want the newer %+v", got, revoked)
	}
}

func TestGameModeNeverTypesIntoAGameThatIsGone(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.discovery.Conn.PID = 4242
	h.run(t)
	h.playHostedMatch(t, "-ar", 5)
	waitFor(t, "the match", func() bool { return strings.HasPrefix(h.svc.State().Status, "The match started.") })
	// The game signs out and can't be found again.
	h.discovery.Set(dto.GameConnection{}, errors.New("Warcraft III isn't running."))
	h.session.Push(dto.GameEvent{MessageType: "LoggedOut"})
	h.skew.Store(int64(time.Minute))
	waitFor(t, "the hint", func() bool { return h.svc.State().ConnectionHint == "Warcraft III isn't running." })
	// Give a due line a tick to be typed, if it were going to be.
	time.Sleep(1200 * time.Millisecond)
	if got := h.keyboard.Typed(); len(got) != 0 {
		t.Fatalf("typed %q into a game that was gone", got)
	}
}
