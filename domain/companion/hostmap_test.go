package companion_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	hf5File = "/detected/Maps/Download/Legion_TD_11.4c-hf5_Team_OZE.w3x"
	hf9File = "/detected/Maps/Download/Legion_TD_11.4c-hf9_Team_OZE.w3x"
	hf1File = "/detected/Maps/Download/Legion_TD_11.2c-hf1_TeamOZE.w3x"
)

// createdOn lists the files of every CreateLobby sent, as the game saw them.
func (h *harness) createdOn() []string {
	var files []string
	for _, c := range h.session.Sent() {
		if c.Message == "CreateLobby" {
			files = append(files, c.Payload["filename"].(string))
		}
	}
	return files
}

// hostOn turns Auto-host on for the chosen map, with Keep maps up to date
// off so nothing downloads by itself, and waits for the first lobby.
func (h *harness) hostOn(t *testing.T, mapPath string, useNewest bool) dto.FollowState {
	t.Helper()
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	settings := dto.HostSettings{MapPath: mapPath, MapName: "Legion TD", GameName: "LTD", UseNewest: useNewest}
	on := true
	state, err := h.svc.ConfigureHost(context.Background(), dto.HostConfig{Settings: &settings, Enabled: &on, FromMenus: true})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "CreateLobby", func() bool { return len(h.createdOn()) == 1 })
	return state
}

// playAndReturn plays the open lobby's match and goes back to the menus.
func (h *harness) playAndReturn() {
	for _, ev := range []dto.GameEvent{
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "GAME_LOBBY"}},
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "LOADING_SCREEN"}},
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "DISABLED_SCREEN"}},
		{MessageType: "ExitedGame"},
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "SCORE_SCREEN"}},
		{MessageType: "MultiplayerGameLeave"},
		{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "MULTIPLAYER"}},
	} {
		h.session.Push(ev)
	}
}

func TestAutoHostHostsTheNewestInstalledVersionOfATrackedMap(t *testing.T) {
	h := newMapHarness(t, mapPrefs(false))
	h.library.SetInstalled(dto.InstalledMap{Version: "11.2c-hf1", Path: hf1File, Modified: time.Now()},
		dto.InstalledMap{Version: "11.4c-hf9", Path: hf9File, Modified: time.Now().Add(-time.Hour)},
		dto.InstalledMap{Version: "11.4c-hf5", Path: hf5File, Modified: time.Now().Add(-2 * time.Hour)})
	h.run(t)
	state := h.hostOn(t, hf5File, true)
	if got := h.createdOn(); got[0] != "GAME:"+hf9File {
		t.Fatalf("created on %q", got)
	}
	want := dto.HostMap{Path: hf9File, Tracked: "Legion TD", Version: "11.4c-hf9", ChosenVersion: "11.4c-hf5", NewestVersion: "11.4c-hf9", Newest: true}
	if state.Host.Map != want {
		t.Fatalf("map = %+v", state.Host.Map)
	}
	if !logged(h.svc.State(), "Creating your lobby “LTD” on Legion TD 11.4c-hf9.") {
		t.Fatal("Activity doesn't name the hosted version")
	}
}

func TestANewlyDownloadedVersionIsHostedInTheNextLobby(t *testing.T) {
	h := newMapHarness(t, mapPrefs(false))
	h.run(t)
	h.hostOn(t, hf5File, true)
	if got := h.createdOn(); got[0] != "GAME:"+hf5File {
		t.Fatalf("created on %q", got)
	}
	// Map updates finished downloading hf9 during the match.
	h.library.SetInstalled(dto.InstalledMap{Version: "11.4c-hf9", Path: hf9File, Modified: time.Now()},
		dto.InstalledMap{Version: "11.4c-hf5", Path: hf5File, Modified: time.Now().Add(-time.Hour)})
	h.playAndReturn()
	waitFor(t, "the next lobby", func() bool { return len(h.createdOn()) == 2 })
	if got := h.createdOn(); got[1] != "GAME:"+hf9File {
		t.Fatalf("created on %q", got)
	}
	if m := h.svc.State().Host.Map; m.Version != "11.4c-hf9" || !m.Newest {
		t.Fatalf("map = %+v", m)
	}
}

func TestADownloadedMapIsShownAsTheNextOneHosted(t *testing.T) {
	h := newMapHarness(t, mapPrefs(true))
	_, _ = h.svc.ConfigureHost(context.Background(), dto.HostConfig{Settings: &dto.HostSettings{MapPath: hf5File, MapName: "Legion TD", GameName: "LTD", UseNewest: true}})
	h.run(t)
	waitFor(t, "the download", func() bool { return h.svc.State().MapUpdates.Maps[0].Phase == dto.MapDone })
	waitFor(t, "the newest map", func() bool { return h.svc.State().Host.Map.Version == "11.4c-hf9" })
}

func TestAnUntrackedMapIsHostedAsChosen(t *testing.T) {
	h := newMapHarness(t, mapPrefs(false))
	h.run(t)
	state := h.hostOn(t, "/detected/Maps/Legion TD.w3x", true)
	if got := h.createdOn(); got[0] != "GAME:/detected/Maps/Legion TD.w3x" {
		t.Fatalf("created on %q", got)
	}
	if want := (dto.HostMap{Path: "/detected/Maps/Legion TD.w3x"}); state.Host.Map != want {
		t.Fatalf("map = %+v", state.Host.Map)
	}
	h.playAndReturn()
	waitFor(t, "the next lobby", func() bool { return len(h.createdOn()) == 2 })
	if got := h.createdOn(); got[1] != "GAME:/detected/Maps/Legion TD.w3x" {
		t.Fatalf("created on %q", got)
	}
}

func TestWithTheOptionOffTheChosenVersionIsHosted(t *testing.T) {
	h := newMapHarness(t, mapPrefs(false))
	h.library.SetInstalled(dto.InstalledMap{Version: "11.4c-hf9", Path: hf9File}, dto.InstalledMap{Version: "11.4c-hf5", Path: hf5File})
	h.run(t)
	state := h.hostOn(t, hf5File, false)
	h.playAndReturn()
	waitFor(t, "the next lobby", func() bool { return len(h.createdOn()) == 2 })
	if got := h.createdOn(); !slices.Equal(got, []string{"GAME:" + hf5File, "GAME:" + hf5File}) {
		t.Fatalf("created on %q", got)
	}
	want := dto.HostMap{Path: hf5File, Tracked: "Legion TD", Version: "11.4c-hf5", ChosenVersion: "11.4c-hf5", NewestVersion: "11.4c-hf9"}
	if state.Host.Map != want {
		t.Fatalf("map = %+v", state.Host.Map)
	}
}

func TestHostMapAnswersTheSetupDialog(t *testing.T) {
	h := newMapHarness(t, mapPrefs(false))
	h.library.SetInstalled(dto.InstalledMap{Version: "11.4c-hf9", Path: hf9File}, dto.InstalledMap{Version: "11.4c-hf5", Path: hf5File})
	ctx := context.Background()
	got := h.svc.HostMap(ctx, dto.HostSettings{MapPath: hf5File, UseNewest: true})
	if got.Tracked != "Legion TD" || got.Path != hf9File || got.ChosenVersion != "11.4c-hf5" || got.NewestVersion != "11.4c-hf9" || !got.Newest {
		t.Fatalf("map = %+v", got)
	}
	if got := h.svc.HostMap(ctx, dto.HostSettings{MapPath: hf5File}); got.Path != hf5File || got.Newest || got.NewestVersion != "11.4c-hf9" {
		t.Fatalf("option off: map = %+v", got)
	}
	if got := h.svc.HostMap(ctx, dto.HostSettings{MapPath: "/detected/Maps/ff.w3x", UseNewest: true}); got != (dto.HostMap{Path: "/detected/Maps/ff.w3x"}) {
		t.Fatalf("untracked: map = %+v", got)
	}
}
