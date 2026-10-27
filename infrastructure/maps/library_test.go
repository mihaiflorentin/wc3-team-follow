package maps_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/maps"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func touch(t *testing.T, root string, files ...string) {
	t.Helper()
	for i, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("map"), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		_ = os.Chtimes(p, at, at)
	}
}

func names(entries []dto.MapEntry) []string {
	var out []string
	for _, e := range entries {
		n := e.Name
		if e.IsFolder {
			n += "/"
		}
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func fixture(t *testing.T) string {
	root := t.TempDir()
	touch(t, root,
		"(2)Lost Temple.w3m",
		"Legion TD x20.W3X",
		"notes.txt",
		".hidden.w3x",
		"Download/Green Circle TD.w3x",
		"Download/bfa6cfad1d4a975f6dae00661e08092212b6d1f7/(4)LostTemple_LV.w3x",
		"Download/a2da0c018f3a9ff7e28418aad71c3d690988a936/one.w3x",
		"Download/a2da0c018f3a9ff7e28418aad71c3d690988a936/two.w3x",
		"Download/Tower Defense/Legion TD Mega.w3x",
	)
	return root
}

func TestBrowseListsFoldersAndMaps(t *testing.T) {
	root := fixture(t)
	got, err := maps.New().Browse(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"(2)Lost Temple", "Download/", "Legion TD x20"}; !slices.Equal(names(got.Entries), want) {
		t.Fatalf("got %v, want %v", names(got.Entries), want)
	}
	for _, e := range got.Entries {
		if e.Name == "Legion TD x20" && (e.Modified.IsZero() || e.Size != 3 || filepath.Base(e.Path) != "Legion TD x20.W3X") {
			t.Errorf("entry = %+v", e)
		}
	}
}

func TestBrowseShowsSingleMapDownloadFoldersAsMaps(t *testing.T) {
	root := fixture(t)
	got, err := maps.New().Browse(context.Background(), root, "Download")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"(4)LostTemple_LV", "Green Circle TD", "Tower Defense/", "a2da0c018f3a9ff7e28418aad71c3d690988a936/"}
	if !slices.Equal(names(got.Entries), want) {
		t.Fatalf("got %v, want %v", names(got.Entries), want)
	}
	if got.Dir != "Download" {
		t.Errorf("dir = %q", got.Dir)
	}
}

func TestBrowseStaysInsideTheRoot(t *testing.T) {
	root := fixture(t)
	if _, err := maps.New().Browse(context.Background(), root, "../.."); !errors.Is(err, maps.ErrOutsideRoot) {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchIsRecursiveCaseInsensitiveAndMatchesAllWords(t *testing.T) {
	root := fixture(t)
	got, err := maps.New().Search(context.Background(), root, "legion td")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Legion TD Mega", "Legion TD x20"}; !slices.Equal(names(got.Entries), want) {
		t.Fatalf("got %v", names(got.Entries))
	}
	for _, e := range got.Entries {
		if e.Name == "Legion TD Mega" && e.Dir != "Download/Tower Defense" {
			t.Errorf("dir = %q", e.Dir)
		}
	}
	empty, _ := maps.New().Search(context.Background(), root, "  ")
	if len(empty.Entries) != 0 {
		t.Error("empty query returned results")
	}
	hidden, _ := maps.New().Search(context.Background(), root, "hidden")
	if len(hidden.Entries) != 0 {
		t.Error("found a hidden map")
	}
}
