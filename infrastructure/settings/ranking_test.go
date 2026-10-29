package settings_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/settings"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func rankingFile(dir string) string { return filepath.Join(dir, "settings", "ranking.json") }

func TestRankingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	played := time.Date(2026, 10, 30, 20, 15, 0, 0, time.UTC)
	entries := []dto.RankingEntry{
		{Tag: "Jaina#2214", Wins: 1, Losses: 5, LastPlayed: played.Add(-time.Hour)},
		{Tag: "Arthas#1123", Wins: 7, Losses: 2, LastPlayed: played},
	}
	if err := settings.NewRanking(dir).Save(entries); err != nil {
		t.Fatal(err)
	}
	got, err := settings.NewRanking(dir).Load()
	if err != nil || len(got) != 2 || got[0].Tag != "Arthas#1123" || got[0].Wins != 7 || got[0].Losses != 2 ||
		!got[0].LastPlayed.Equal(played) || got[1].Tag != "Jaina#2214" || got[1].Losses != 5 {
		t.Fatalf("loaded %+v, %v", got, err)
	}
}

func TestRankingSaveKeepsTheMostRecentMaximum(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var entries []dto.RankingEntry
	total := dto.MaxRankingEntries + 20
	for i := range total {
		entries = append(entries, dto.RankingEntry{Tag: fmt.Sprintf("Player%d#%d", i, 10000+i), Wins: 1, LastPlayed: start.Add(time.Duration(i) * time.Minute)})
	}
	if err := settings.NewRanking(dir).Save(entries); err != nil {
		t.Fatal(err)
	}
	got, err := settings.NewRanking(dir).Load()
	if err != nil || len(got) != dto.MaxRankingEntries || got[0].Tag != fmt.Sprintf("Player%d#%d", total-1, 10000+total-1) || got[len(got)-1].Tag != "Player20#10020" {
		t.Fatalf("loaded %d, %v", len(got), err)
	}
}

func TestRankingDamagedFileLoadsEmptyAndStaysInPlace(t *testing.T) {
	dir := t.TempDir()
	if got, err := settings.NewRanking(dir).Load(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("missing file: %v, %v", got, err)
	}
	file := rankingFile(dir)
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	for _, content := range []string{"not json", `{"entries":"x"}`, `[1,2]`, ``} {
		_ = os.WriteFile(file, []byte(content), 0o600)
		if got, err := settings.NewRanking(dir).Load(); err != nil || got == nil || len(got) != 0 {
			t.Errorf("%q: %v, %v", content, got, err)
		}
		if left, err := os.ReadFile(file); err != nil || string(left) != content {
			t.Errorf("%q: the damaged file was changed: %q, %v", content, left, err)
		}
	}
	// Entries without a BattleTag are dropped and counts can't be negative.
	_ = os.WriteFile(file, []byte(`{"entries":[{"tag":"","wins":3},{"tag":"Thrall#1820","wins":-4,"losses":2,"lastPlayed":"2026-10-28T10:00:00Z"}]}`), 0o600)
	got, err := settings.NewRanking(dir).Load()
	if err != nil || len(got) != 1 || got[0].Tag != "Thrall#1820" || got[0].Wins != 0 || got[0].Losses != 2 {
		t.Fatalf("partly valid file: %v, %v", got, err)
	}
}

func TestRankingOversizedFileLoadsEmptyAndStaysInPlace(t *testing.T) {
	dir := t.TempDir()
	file := rankingFile(dir)
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	big := `{"entries":[{"tag":"Arthas#1123","wins":1}],"junk":"` + strings.Repeat("x", 600*1024) + `"}`
	_ = os.WriteFile(file, []byte(big), 0o600)
	if got, err := settings.NewRanking(dir).Load(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("oversized file: %d entries, %v", len(got), err)
	}
	if info, err := os.Stat(file); err != nil || info.Size() != int64(len(big)) {
		t.Fatalf("the oversized file was changed: %v, %v", info, err)
	}
}

func TestRankingSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	store := settings.NewRanking(dir)
	if err := store.Save([]dto.RankingEntry{{Tag: "Arthas#1123", Wins: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save([]dto.RankingEntry{{Tag: "Arthas#1123", Wins: 2}}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "settings"))
	if len(entries) != 1 || entries[0].Name() != "ranking.json" {
		t.Fatalf("settings folder = %v; want only ranking.json and no temporary file", entries)
	}
	if info, err := os.Stat(rankingFile(dir)); err != nil || info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("file mode = %v, %v", info.Mode(), err)
	}
	if got, _ := store.Load(); len(got) != 1 || got[0].Wins != 2 {
		t.Fatalf("loaded %+v; the second save replaces the first", got)
	}
}

func TestRankingFailedSaveLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	store := settings.NewRanking(dir)
	if err := store.Save([]dto.RankingEntry{{Tag: "Arthas#1123", Wins: 1}}); err != nil {
		t.Fatal(err)
	}
	// A folder where the file belongs makes the final rename fail.
	other := t.TempDir()
	_ = os.MkdirAll(rankingFile(other), 0o700)
	if err := settings.NewRanking(other).Save([]dto.RankingEntry{{Tag: "Arthas#1123", Wins: 9}}); err == nil {
		t.Fatal("saving over a folder succeeded")
	}
	if left, _ := os.ReadDir(filepath.Join(other, "settings")); len(left) != 1 {
		t.Fatalf("a temporary file was left behind: %v", left)
	}
	if got, _ := store.Load(); len(got) != 1 || got[0].Wins != 1 {
		t.Fatalf("loaded %+v", got)
	}
}

func TestRankingClear(t *testing.T) {
	dir := t.TempDir()
	store := settings.NewRanking(dir)
	if err := store.Clear(); err != nil {
		t.Fatalf("clearing nothing: %v", err)
	}
	if err := store.Save([]dto.RankingEntry{{Tag: "Arthas#1123", Wins: 1, LastPlayed: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(); err != nil || len(got) != 0 {
		t.Fatalf("after Clear: %v, %v", got, err)
	}
}
