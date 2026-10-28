package settings_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/settings"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	tags := []string{"One#1234", "Two#2345", "Three#3456"}
	if err := settings.New(dir).Save(dto.Preferences{Tags: tags, IncludeTeam: false, JoinHostTeam: true}); err != nil {
		t.Fatal(err)
	}
	got, err := settings.New(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Tags, tags) || got.IncludeTeam || !got.JoinHostTeam {
		t.Fatalf("loaded %+v", got)
	}
	var raw map[string]any
	content, _ := os.ReadFile(filepath.Join(dir, "settings", "friends.json"))
	if err := json.Unmarshal(content, &raw); err != nil || len(raw) != 2 {
		t.Fatalf("file holds more than tags and includeTeam: %s", content)
	}
}

func TestAskPasswordIsSaved(t *testing.T) {
	dir := t.TempDir()
	if err := settings.New(dir).Save(dto.Preferences{Tags: []string{}, IncludeTeam: true, AskPassword: true}); err != nil {
		t.Fatal(err)
	}
	if got, err := settings.New(dir).Load(); err != nil || !got.AskPassword {
		t.Fatalf("loaded %+v, %v", got, err)
	}
}

func TestJoinHostTeamIsOnUnlessSavedOff(t *testing.T) {
	dir := t.TempDir()
	if got, err := settings.New(dir).Load(); err != nil || !got.JoinHostTeam {
		t.Fatalf("no file: loaded %+v, %v", got, err)
	}
	// A file saved before the option existed has no joinHostTeam.
	_ = os.MkdirAll(filepath.Join(dir, "settings"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "settings", "friends.json"), []byte(`{"tags":[],"includeTeam":true}`), 0o600)
	if got, err := settings.New(dir).Load(); err != nil || !got.JoinHostTeam {
		t.Fatalf("older file: loaded %+v, %v", got, err)
	}
	if err := settings.New(dir).Save(dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: false}); err != nil {
		t.Fatal(err)
	}
	if got, err := settings.New(dir).Load(); err != nil || got.JoinHostTeam {
		t.Fatalf("saved off: loaded %+v, %v", got, err)
	}
}

func TestSaveLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	store := settings.New(dir)
	_ = store.Save(dto.Preferences{Tags: []string{"One#1234"}, IncludeTeam: true})
	if err := store.Save(dto.Preferences{IncludeTeam: false}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "settings"))
	if len(entries) != 1 || entries[0].Name() != "friends.json" {
		t.Fatalf("settings folder = %v", entries)
	}
	got, _ := store.Load()
	if len(got.Tags) != 0 || got.IncludeTeam {
		t.Fatalf("loaded %+v", got)
	}
}

func TestMissingFileUsesDefaults(t *testing.T) {
	got, err := settings.New(t.TempDir()).Load()
	if err != nil || len(got.Tags) != 0 || !got.IncludeTeam {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestMalformedFileFailsWithoutBeingOverwritten(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "settings", "friends.json")
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	for _, content := range []string{"not json", `{"tags":[]}`, `{"includeTeam":true}`, `{"tags":"x","includeTeam":true}`} {
		_ = os.WriteFile(file, []byte(content), 0o600)
		if _, err := settings.New(dir).Load(); !errors.Is(err, settings.ErrUnreadable) {
			t.Errorf("%s: err = %v", content, err)
		}
		if got, _ := os.ReadFile(file); string(got) != content {
			t.Errorf("file was changed to %s", got)
		}
	}
}
