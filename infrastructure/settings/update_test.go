package settings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/settings"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestUpdateSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := settings.New(dir).Save(dto.Preferences{Tags: []string{}, IncludeTeam: true, CheckUpdates: false, SkippedVersion: "0.8.0"}); err != nil {
		t.Fatal(err)
	}
	got, err := settings.New(dir).Load()
	if err != nil || got.CheckUpdates || got.SkippedVersion != "0.8.0" {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	if err := settings.New(dir).Save(dto.Preferences{Tags: []string{}, IncludeTeam: true, CheckUpdates: true}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "settings", "friends.json"))
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil || raw["checkUpdates"] != nil || raw["skippedVersion"] != nil {
		t.Fatalf("defaults written to the file: %s", content)
	}
	if got, err := settings.New(dir).Load(); err != nil || !got.CheckUpdates || got.SkippedVersion != "" {
		t.Fatalf("loaded %+v, %v", got, err)
	}
}

func TestOlderFileAndNoFileLoadWithUpdateChecksOn(t *testing.T) {
	dir := t.TempDir()
	if got, err := settings.New(dir).Load(); err != nil || !got.CheckUpdates {
		t.Fatalf("no file: loaded %+v, %v", got, err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "settings"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "settings", "friends.json"), []byte(`{"tags":[],"includeTeam":true}`), 0o600)
	if got, err := settings.New(dir).Load(); err != nil || !got.CheckUpdates || got.SkippedVersion != "" {
		t.Fatalf("older file: loaded %+v, %v", got, err)
	}
}
