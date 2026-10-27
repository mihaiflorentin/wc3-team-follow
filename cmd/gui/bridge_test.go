package gui_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/cmd/gui"
	mockcompanion "github.com/mihaiflorentin/wc3-team-follow/mock/companion"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestBridgePassesCallsToTheCompanion(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Connected: true}}
	b := gui.NewBridge(c, "1.2.3", "/opt/wc3-team-follow", "")

	on := true
	if s, err := b.Configure(dto.FollowConfig{Enabled: &on, FromMenus: true}); err != nil || !s.Connected {
		t.Fatalf("Configure = %+v, %v", s, err)
	}
	if c.LastConfig.Enabled == nil || !*c.LastConfig.Enabled || !c.LastConfig.FromMenus {
		t.Fatalf("config = %+v", c.LastConfig)
	}
	if _, err := b.SubmitPassword("secret"); err != nil || c.LastPassword != "secret" {
		t.Fatalf("password not passed on: %v", err)
	}
	if !b.State().Connected || b.ExecutablePath() != "/opt/wc3-team-follow" {
		t.Fatal("state or path not passed on")
	}
}

func TestBridgeReturnsCompanionErrors(t *testing.T) {
	c := &mockcompanion.Companion{Err: errors.New("Return to Custom Games before starting.")}
	if _, err := gui.NewBridge(c, "", "", "").Configure(dto.FollowConfig{}); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestDiagnosticsAddTheAppVersion(t *testing.T) {
	c := &mockcompanion.Companion{Report: dto.Diagnostics{Version: "3.0.1.24371", Platform: "linux/amd64"}}
	text, err := gui.NewBridge(c, "1.2.3", "", "/logs").Diagnostics()
	if err != nil {
		t.Fatal(err)
	}
	var report dto.Diagnostics
	if err := json.Unmarshal([]byte(text), &report); err != nil || report.AppVersion != "1.2.3" || report.Platform != "linux/amd64" || report.LogFolder != "/logs" {
		t.Fatalf("report = %s, %v", text, err)
	}
}

func TestOpenLogFolderNeedsFileLogging(t *testing.T) {
	if err := gui.NewBridge(&mockcompanion.Companion{}, "", "", "").OpenLogFolder(); err == nil {
		t.Fatal("opened a log folder with file logging off")
	}
}

func TestBridgePassesHostAndMapCalls(t *testing.T) {
	c := &mockcompanion.Companion{Listing: dto.MapListing{Entries: []dto.MapEntry{{Name: "Legion TD"}}}}
	b := gui.NewBridge(c, "", "", "")
	on := true
	if _, err := b.ConfigureHost(dto.HostConfig{Enabled: &on}); err != nil || c.LastHost.Enabled == nil {
		t.Fatalf("host config not passed on: %v", err)
	}
	if got := b.MapsFolders(); got == nil {
		t.Fatal("MapsFolders returned nil instead of an empty list")
	}
	if _, err := b.SetMapsFolder("/maps"); err != nil || c.LastFolder != "/maps" {
		t.Fatal("folder not passed on")
	}
	if l, err := b.BrowseMaps("Download"); err != nil || l.Dir != "Download" || len(l.Entries) != 1 {
		t.Fatalf("browse = %+v, %v", l, err)
	}
	if _, err := b.SearchMaps("legion"); err != nil || c.LastQuery != "legion" {
		t.Fatal("search not passed on")
	}
	if _, err := b.ChooseMapsFolder(""); err == nil {
		t.Fatal("folder picker ran outside the app")
	}
}

func TestBridgePassesCloseGamesOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{GameCount: 0, Status: "Closed 2 copies of Warcraft III."}}
	s, err := gui.NewBridge(c, "", "", "").CloseGames()
	if err != nil || c.CloseCalls != 1 || s.Status != c.Current.Status {
		t.Fatalf("CloseGames = %+v, %v (calls %d)", s, err, c.CloseCalls)
	}
	c.Err = errors.New("Warcraft III couldn't be closed. Close it in your task manager.")
	if _, err := gui.NewBridge(c, "", "", "").CloseGames(); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("err = %v", err)
	}
}
