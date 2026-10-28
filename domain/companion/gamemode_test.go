package companion_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// playHostedMatch turns Auto-host on with Game mode and plays its lobby
// into a match.
func (h *harness) playHostedMatch(t *testing.T, command string) {
	t.Helper()
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	settings := dto.HostSettings{MapPath: "/detected/Maps/Legion TD.w3x", MapName: "Legion TD", GameName: "LTD", GameMode: command}
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
	h.playHostedMatch(t, "-ar")
	waitFor(t, "the typed note", func() bool { return h.svc.State().Status == "Typed -ar in the game chat." })
	if got := h.keyboard.Typed(); !slices.Equal(got, []string{"-ar"}) || !slices.Equal(h.keyboard.PIDs, []int{4242}) {
		t.Fatalf("typed %q into %v", got, h.keyboard.PIDs)
	}
}

func TestGameModeSaysWhyTypingFailed(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.keyboard.Err = errors.New("Warcraft III wasn't in front, so typing stopped.")
	h.playHostedMatch(t, "-ar")
	want := "Game mode didn't type -ar. Warcraft III wasn't in front, so typing stopped."
	waitFor(t, "the failure note", func() bool { return h.svc.State().Status == want })
	if got := h.keyboard.Typed(); len(got) != 1 {
		t.Fatalf("typed %q; it must not retry", got)
	}
}

func TestGameModeDoesNotTypeWithoutPermission(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.keyboard.Current = dto.KeyboardStatus{Supported: true, Reason: "Allow Team Follow to type in Accessibility settings."}
	h.playHostedMatch(t, "-ar")
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
