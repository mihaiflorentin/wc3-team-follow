package discovery

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeReplay writes a fake LastReplay.w3g of size bytes into
// <base>/<account>/Replays, changed at modified.
func writeReplay(t *testing.T, base, account string, size int, modified time.Time) string {
	t.Helper()
	dir := filepath.Join(filepath.FromSlash(base), account, "Replays")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, lastReplayName)
	if err := os.WriteFile(file, bytes.Repeat([]byte{0}, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, modified, modified); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestNewestReplayPicksTheNewestAcrossAccountsAndFolders(t *testing.T) {
	home := t.TempDir()
	folders := darwinReplayFolders(home)
	old := time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)
	writeReplay(t, folders[0], "11111111", 10, old)
	newest := writeReplay(t, folders[1], "22222222", 42, old.Add(time.Hour))
	// An account folder without replays, and a stray file, are skipped.
	mkdirs(t, filepath.ToSlash(filepath.Join(folders[0], "33333333")))
	if err := os.WriteFile(filepath.Join(folders[0], "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := newestReplay(folders)
	if err != nil || got.Path != newest || got.Size != 42 || !got.Modified.Equal(old.Add(time.Hour)) {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestNewestReplayInWindowsDocumentsAndOneDrive(t *testing.T) {
	docs, onedrive := t.TempDir(), t.TempDir()
	folders := documentsReplayFolders([]string{docs, "", filepath.Join(onedrive, "Documents")})
	if len(folders) != 2 || folders[0] != filepath.Join(docs, "Warcraft III", "BattleNet") {
		t.Fatalf("folders = %v", folders)
	}
	at := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	writeReplay(t, folders[0], "11111111", 1, at)
	want := writeReplay(t, folders[1], "11111111", 2, at.Add(time.Minute))
	if got, err := newestReplay(folders); err != nil || got.Path != want {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestLinuxReplayFoldersInsideWinePrefixes(t *testing.T) {
	home := filepath.ToSlash(t.TempDir())
	p := procfs{root: filepath.ToSlash(t.TempDir()), home: home}
	mkdirs(t, home+"/.wine/drive_c/users/me", home+"/Games/battlenet/drive_c/users/steamuser")
	folders := p.replayFolders(context.Background(), nil)
	want := path.Join(home, "Games/battlenet/drive_c/users/steamuser/Documents/Warcraft III/BattleNet")
	found := false
	for _, f := range folders {
		found = found || filepath.ToSlash(f) == want
	}
	if !found {
		t.Fatalf("folders = %v", folders)
	}
	file := writeReplay(t, want, "44444444", 7, time.Now())
	if got, err := newestReplay(folders); err != nil || got.Path != file {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestNewestReplayWithoutOneIsAnError(t *testing.T) {
	if _, err := newestReplay([]string{t.TempDir(), "", filepath.Join(t.TempDir(), "missing")}); !errors.Is(err, ErrNoReplay) {
		t.Fatalf("err = %v", err)
	}
}

func TestLastReplayNeverLogsTheAccountFolder(t *testing.T) {
	base := t.TempDir()
	file := writeReplay(t, base, "98765432", 5, time.Now())
	var log bytes.Buffer
	d := newDiscovery(fakePlatform{replays: []string{base}}, slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	got, err := d.LastReplay(context.Background())
	if err != nil || got.Path != file {
		t.Fatalf("got %+v, %v", got, err)
	}
	if strings.Contains(log.String(), "98765432") || !strings.Contains(log.String(), "found=true") {
		t.Fatalf("log = %q", log.String())
	}
}
