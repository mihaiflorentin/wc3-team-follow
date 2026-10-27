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
	b := gui.NewBridge(c, "1.2.3", "/opt/wc3-team-follow")

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
	if _, err := gui.NewBridge(c, "", "").Configure(dto.FollowConfig{}); err == nil || err.Error() != c.Err.Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestDiagnosticsAddTheAppVersion(t *testing.T) {
	c := &mockcompanion.Companion{Report: dto.Diagnostics{Version: "3.0.1.24371", Platform: "linux/amd64"}}
	text, err := gui.NewBridge(c, "1.2.3", "").Diagnostics()
	if err != nil {
		t.Fatal(err)
	}
	var report dto.Diagnostics
	if err := json.Unmarshal([]byte(text), &report); err != nil || report.AppVersion != "1.2.3" || report.Platform != "linux/amd64" {
		t.Fatalf("report = %s, %v", text, err)
	}
}
