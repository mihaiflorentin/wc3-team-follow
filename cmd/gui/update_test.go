package gui_test

import (
	"errors"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/cmd/gui"
	mockcompanion "github.com/mihaiflorentin/wc3-team-follow/mock/companion"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const page = "https://github.com/mihaiflorentin/wc3-team-follow/releases"

func updateBridge(c *mockcompanion.Companion) (*gui.Bridge, *[]string) {
	b := gui.NewBridge(c, "0.7.0", "", "")
	var opened []string
	b.SetOpenURL(func(link string) error { opened = append(opened, link); return nil })
	return b, &opened
}

func TestBridgePassesWhatsNewAndTheChangelogOn(t *testing.T) {
	c := &mockcompanion.Companion{ChangelogSections: []dto.ChangelogSection{{Version: "0.8.0"}, {Version: "0.7.0"}}}
	b, _ := updateBridge(c)
	if b.DismissWhatsNew(); c.DismissCalls != 1 {
		t.Fatal("DismissWhatsNew not passed on")
	}
	if got := b.Changelog(); len(got) != 2 || got[0].Version != "0.8.0" {
		t.Fatalf("Changelog = %+v", got)
	}
}

func TestBridgePassesUpdateCallsOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Update: dto.UpdateState{Phase: dto.UpdateChecking}}}
	b, _ := updateBridge(c)
	if s, err := b.CheckForUpdates(); err != nil || c.CheckCalls != 1 || s.Update.Phase != dto.UpdateChecking {
		t.Fatalf("CheckForUpdates = %+v, %v", s.Update, err)
	}
	if b.UpdateLater(); c.LaterCalls != 1 {
		t.Fatal("Later not passed on")
	}
	if _, err := b.SkipUpdate(); err != nil || c.SkipCalls != 1 {
		t.Fatalf("Skip not passed on: %v", err)
	}
	if _, err := b.SetCheckUpdates(false); err != nil || c.LastConfig.CheckUpdates == nil || *c.LastConfig.CheckUpdates {
		t.Fatalf("setting not passed on: %+v, %v", c.LastConfig, err)
	}
	c.Err = errors.New("Updates are off in development builds.")
	if _, err := b.CheckForUpdates(); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateInstallsInTheAppAndCancelAndDoneArePassedOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Update: dto.UpdateState{Phase: dto.UpdateDownloading}}}
	b, opened := updateBridge(c)
	if s, err := b.Update(); err != nil || c.InstallCalls != 1 || s.Update.Phase != dto.UpdateDownloading || len(*opened) != 0 {
		t.Fatalf("Update = %+v, %v, opened %v", s.Update, err, *opened)
	}
	if b.CancelUpdate(); c.CancelCalls != 1 {
		t.Fatal("Cancel not passed on")
	}
	if b.DismissUpdateNotes(); c.DismissNotesCalls != 1 {
		t.Fatal("Done not passed on")
	}
	c.Current.Update = dto.UpdateState{ReleasesURL: page}
	if err := b.OpenReleasesPage(); err != nil || len(*opened) != 1 || (*opened)[0] != page {
		t.Fatalf("OpenReleasesPage = %v, opened %v", err, *opened)
	}
}

func TestTheBridgeOpensOnlyGitHubPages(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{Update: dto.UpdateState{
		Release: &dto.UpdateRelease{URL: "javascript:alert(1)"}, ReleasesURL: "http://github.com/releases"}}}
	b, opened := updateBridge(c)
	if err := b.OpenReleasesPage(); err == nil || len(*opened) != 0 {
		t.Fatalf("opened %v", *opened)
	}
}
