package gui_test

import (
	"testing"

	mockcompanion "github.com/mihaiflorentin/wc3-team-follow/mock/companion"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestBridgePassesMapUpdateCallsOn(t *testing.T) {
	c := &mockcompanion.Companion{Current: dto.FollowState{MapUpdates: dto.MapUpdatesState{Checking: true}}}
	b, _ := updateBridge(c)
	if s, err := b.CheckMaps(); err != nil || c.MapChecks != 1 || !s.MapUpdates.Checking {
		t.Fatalf("CheckMaps = %+v, %v", s.MapUpdates, err)
	}
	if _, err := b.SetMapUpdates(false); err != nil || c.LastConfig.MapUpdates == nil || *c.LastConfig.MapUpdates {
		t.Fatalf("setting not passed on: %+v, %v", c.LastConfig, err)
	}
}

func TestOpenMapReleasePageOpensOnlyTheStatesGitHubPage(t *testing.T) {
	release := "https://github.com/team-oze/Legion-TD/releases/tag/v11.4c-hf9"
	c := &mockcompanion.Companion{Current: dto.FollowState{MapUpdates: dto.MapUpdatesState{Maps: []dto.MapUpdate{
		{Name: "Legion TD", PageURL: release},
		{Name: "Elsewhere", PageURL: "https://example.com/map"},
	}}}}
	b, opened := updateBridge(c)
	if err := b.OpenMapReleasePage("Legion TD"); err != nil || len(*opened) != 1 || (*opened)[0] != release {
		t.Fatalf("err = %v, opened %v", err, *opened)
	}
	if err := b.OpenMapReleasePage("Elsewhere"); err == nil {
		t.Fatal("opened a page outside GitHub")
	}
	if err := b.OpenMapReleasePage("https://github.com/evil"); err == nil {
		t.Fatal("opened an address from the page")
	}
	if len(*opened) != 1 {
		t.Fatalf("opened %v", *opened)
	}
}
