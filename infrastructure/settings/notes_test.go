package settings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/settings"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestPlayerNotesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	written := time.Date(2026, 10, 28, 20, 15, 0, 0, time.UTC)
	notes := map[string]dto.PlayerNote{
		"arthas#1123": {Text: "Carries in Legion TD.\nLeaves early when losing.", Updated: written},
		"jaina#2214":  {Text: "Friend of Thrall.", Updated: written.Add(-time.Hour)},
	}
	if err := settings.NewPlayerNotes(dir).Save(notes); err != nil {
		t.Fatal(err)
	}
	got, err := settings.NewPlayerNotes(dir).Load()
	if err != nil || len(got) != 2 || got["arthas#1123"].Text != notes["arthas#1123"].Text ||
		!got["jaina#2214"].Updated.Equal(written.Add(-time.Hour)) {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings", "notes.json")); err != nil {
		t.Fatalf("notes.json: %v", err)
	}
}

func TestPlayerNotesSaveAfterDeletingForgetsTheNote(t *testing.T) {
	dir := t.TempDir()
	store := settings.NewPlayerNotes(dir)
	if err := store.Save(map[string]dto.PlayerNote{"arthas#1123": {Text: "Good healer."}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(map[string]dto.PlayerNote{}); err != nil {
		t.Fatal(err)
	}
	if got, err := settings.NewPlayerNotes(dir).Load(); err != nil || len(got) != 0 {
		t.Fatalf("after deleting: %v, %v", got, err)
	}
}

func TestPlayerNotesMissingOrCorruptFileLoadsEmpty(t *testing.T) {
	dir := t.TempDir()
	if got, err := settings.NewPlayerNotes(dir).Load(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("missing file: %v, %v", got, err)
	}
	file := filepath.Join(dir, "settings", "notes.json")
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	for _, content := range []string{"not json", `{"notes":"x"}`, `[1,2]`, ``} {
		_ = os.WriteFile(file, []byte(content), 0o600)
		if got, err := settings.NewPlayerNotes(dir).Load(); err != nil || got == nil || len(got) != 0 {
			t.Errorf("%q: %v, %v", content, got, err)
		}
	}
}

func TestPlayerNotesLoadCleansEntries(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "settings", "notes.json")
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	long := strings.Repeat("é", dto.MaxNoteLength+20)
	content := `{"notes":{"":{"text":"no tag"},"Thrall#1820":{"text":"  Mixed case key.  "},"blank#1":{"text":"   "},"long#2":{"text":"` + long + `"}}}`
	_ = os.WriteFile(file, []byte(content), 0o600)
	got, err := settings.NewPlayerNotes(dir).Load()
	if err != nil || len(got) != 2 {
		t.Fatalf("loaded %v, %v", got, err)
	}
	if got["thrall#1820"].Text != "Mixed case key." {
		t.Errorf("thrall = %q", got["thrall#1820"].Text)
	}
	if n := len([]rune(got["long#2"].Text)); n != dto.MaxNoteLength {
		t.Errorf("long note kept %d characters", n)
	}
}
