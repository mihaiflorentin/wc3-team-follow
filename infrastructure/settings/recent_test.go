package settings_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/settings"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestRecentPlayersRoundTrip(t *testing.T) {
	dir := t.TempDir()
	seen := time.Date(2026, 10, 28, 20, 15, 0, 0, time.UTC)
	players := []dto.RecentPlayer{
		{Tag: "Arthas#1123", Map: "Castle Fight", LastSeen: seen, FirstSeen: seen.Add(-72 * time.Hour), Lobbies: 7},
		{Tag: "Jaina#2214", Map: "Legion TD", LastSeen: seen.Add(-time.Hour)},
	}
	if err := settings.NewRecentPlayers(dir).Save(players); err != nil {
		t.Fatal(err)
	}
	got, err := settings.NewRecentPlayers(dir).Load()
	if err != nil || len(got) != 2 || got[0].Tag != "Arthas#1123" || got[1].Map != "Legion TD" || !got[0].LastSeen.Equal(seen) ||
		!got[0].FirstSeen.Equal(seen.Add(-72*time.Hour)) || got[0].Lobbies != 7 {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "settings"))
	if len(entries) != 1 || entries[0].Name() != "recent-players.json" {
		t.Fatalf("settings folder = %v", entries)
	}
}

func TestRecentPlayersSaveKeepsTheNewest200(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	var players []dto.RecentPlayer
	for i := range 230 {
		players = append(players, dto.RecentPlayer{Tag: fmt.Sprintf("Player%d#1%03d", i, i), Map: "Castle Fight", LastSeen: start.Add(time.Duration(i) * time.Minute)})
	}
	if err := settings.NewRecentPlayers(dir).Save(players); err != nil {
		t.Fatal(err)
	}
	got, err := settings.NewRecentPlayers(dir).Load()
	if err != nil || len(got) != dto.MaxRecentPlayers || got[0].Tag != "Player229#1229" || got[len(got)-1].Tag != "Player30#1030" {
		t.Fatalf("loaded %d, first %s, last %s, %v", len(got), got[0].Tag, got[len(got)-1].Tag, err)
	}
}

func TestRecentPlayersMissingOrCorruptFileLoadsEmpty(t *testing.T) {
	dir := t.TempDir()
	if got, err := settings.NewRecentPlayers(dir).Load(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("missing file: %v, %v", got, err)
	}
	file := filepath.Join(dir, "settings", "recent-players.json")
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	for _, content := range []string{"not json", `{"players":"x"}`, `[1,2]`} {
		_ = os.WriteFile(file, []byte(content), 0o600)
		if got, err := settings.NewRecentPlayers(dir).Load(); err != nil || got == nil || len(got) != 0 {
			t.Errorf("%s: %v, %v", content, got, err)
		}
	}
	// Entries without a BattleTag are dropped, the rest kept.
	_ = os.WriteFile(file, []byte(`{"players":[{"tag":"","map":"x"},{"tag":"Thrall#1820","map":"Legion TD","lastSeen":"2026-10-28T10:00:00Z"}]}`), 0o600)
	if got, err := settings.NewRecentPlayers(dir).Load(); err != nil || len(got) != 1 || got[0].Tag != "Thrall#1820" {
		t.Fatalf("partly valid file: %v, %v", got, err)
	}
}

func TestRecentPlayersClear(t *testing.T) {
	dir := t.TempDir()
	store := settings.NewRecentPlayers(dir)
	if err := store.Clear(); err != nil {
		t.Fatalf("clearing nothing: %v", err)
	}
	if err := store.Save([]dto.RecentPlayer{{Tag: "Arthas#1123", Map: "Castle Fight", LastSeen: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(); err != nil || len(got) != 0 {
		t.Fatalf("after Clear: %v, %v", got, err)
	}
}
