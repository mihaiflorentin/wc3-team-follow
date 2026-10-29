package gui_test

import (
	"encoding/json"
	"errors"
	"slices"
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

func TestBridgePassesGameModeSettingsAndPermissionRequestsOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Keyboard: dto.KeyboardStatus{Supported: true, Permitted: true}}}
	b := gui.NewBridge(c, "", "", "")
	settings := dto.HostSettings{GameName: "LTD", GameMode: "-ar", GameModeDelaySeconds: 7}
	if _, err := b.ConfigureHost(dto.HostConfig{Settings: &settings}); err != nil || c.LastHost.Settings == nil || *c.LastHost.Settings != settings {
		t.Fatalf("settings not passed on: %+v, %v", c.LastHost.Settings, err)
	}
	if c.PermissionCalls != 0 {
		t.Fatal("asked for the keyboard permission on its own")
	}
	s, err := b.RequestKeyboardPermission()
	if err != nil || c.PermissionCalls != 1 || !s.Keyboard.Permitted {
		t.Fatalf("RequestKeyboardPermission = %+v, %v (calls %d)", s.Keyboard, err, c.PermissionCalls)
	}
}

func TestBridgePassesClearRankingOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Ranking: []dto.RankingEntry{}}}
	s, err := gui.NewBridge(c, "", "", "").ClearRanking()
	if err != nil || c.ClearRankingCalls != 1 || s.Ranking == nil {
		t.Fatalf("ClearRanking = %+v, %v (calls %d)", s.Ranking, err, c.ClearRankingCalls)
	}
	c.Err = errors.New("The ranking couldn't be cleared on this computer. Try again.")
	if _, err := gui.NewBridge(c, "", "", "").ClearRanking(); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("error = %v", err)
	}
}

func TestBridgePassesClearRecentPlayersOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Recent: []dto.RecentPlayer{}}}
	s, err := gui.NewBridge(c, "", "", "").ClearRecentPlayers()
	if err != nil || c.ClearRecentCalls != 1 || s.Recent == nil {
		t.Fatalf("ClearRecentPlayers = %+v, %v (calls %d)", s.Recent, err, c.ClearRecentCalls)
	}
	c.Err = errors.New("Recent players couldn't be cleared on this computer. Try again.")
	if _, err := gui.NewBridge(c, "", "", "").ClearRecentPlayers(); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestBridgePassesBlacklistCallsOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Blacklist: []dto.BlacklistEntry{{Tag: "Arthas#1123"}}}}
	b := gui.NewBridge(c, "", "", "")
	if s, err := b.AddToBlacklist([]string{"Arthas#1123"}); err != nil || len(s.Blacklist) != 1 ||
		!slices.Equal(c.LastConfig.BlacklistAdd, []string{"Arthas#1123"}) {
		t.Fatalf("add one: %+v, %v", c.LastConfig, err)
	}
	if _, err := b.AddToBlacklist([]string{"Player#1234", "Player#5678"}); err != nil || len(c.LastConfig.BlacklistAdd) != 2 {
		t.Fatalf("add many: %+v, %v", c.LastConfig, err)
	}
	if _, err := b.RemoveFromBlacklist("Player#1234"); err != nil || !slices.Equal(c.LastConfig.BlacklistRemove, []string{"Player#1234"}) ||
		c.LastConfig.BlacklistAdd != nil {
		t.Fatalf("remove: %+v, %v", c.LastConfig, err)
	}
	if _, err := b.SetAutoBan(false); err != nil || c.LastConfig.AutoBan == nil || *c.LastConfig.AutoBan {
		t.Fatalf("auto-ban: %+v, %v", c.LastConfig, err)
	}
	if _, err := b.SetBlacklistManualBans(false); err != nil || c.LastConfig.BlacklistManualBans == nil || *c.LastConfig.BlacklistManualBans {
		t.Fatalf("manual bans: %+v, %v", c.LastConfig, err)
	}
	c.Err = errors.New("You can't blacklist yourself.")
	if _, err := b.AddToBlacklist([]string{"Self#1111"}); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("err = %v", err)
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

func TestBridgePassesNotesOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Notes: map[string]dto.PlayerNote{"arthas#1123": {Text: "Good healer."}}}}
	b := gui.NewBridge(c, "", "", "")
	if s, err := b.SetNote("Arthas#1123", "Good healer."); err != nil || s.Notes["arthas#1123"].Text != "Good healer." ||
		c.LastNoteTag != "Arthas#1123" || c.LastNoteText != "Good healer." {
		t.Fatalf("set: %q %q, %v", c.LastNoteTag, c.LastNoteText, err)
	}
	c.LastNoteText = "unchanged"
	if _, err := b.DeleteNote("Jaina#2214"); err != nil || c.LastNoteTag != "Jaina#2214" || c.LastNoteText != "" || c.NoteCalls != 2 {
		t.Fatalf("delete: %q %q (%d calls), %v", c.LastNoteTag, c.LastNoteText, c.NoteCalls, err)
	}
	c.Err = errors.New("A note can be at most 500 characters.")
	if _, err := b.SetNote("Arthas#1123", "x"); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("err = %v", err)
	}
}
