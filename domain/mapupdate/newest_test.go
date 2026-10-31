package mapupdate_test

import (
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/mapupdate"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var legionPattern = dto.TrackedMap{Name: "Legion TD", Repository: "team-oze/Legion-TD", Folder: "Download",
	Pattern: `(?i)^Legion_TD_(?P<version>\d[^_/\\ ]*)_Team_?OZE(?: \(\d+\))?\.w3x$`}

func TestMatchFindsTheTrackedMapOfAChosenFile(t *testing.T) {
	other := dto.TrackedMap{Name: "Other", Pattern: `^Other_(?P<version>[^_]+)\.w3x$`}
	tracked := []dto.TrackedMap{other, legionPattern}
	for name, want := range map[string]string{
		"Legion_TD_11.4c-hf5_Team_OZE.w3x":                "11.4c-hf5",
		"Legion_TD_11.2c-hf1_TeamOZE.w3x":                 "11.2c-hf1",
		"Legion_TD_6.2b_Team_OZE (1).w3x":                 "6.2b",
		"/Maps/Download/Legion_TD_11.4c-hf9_Team_OZE.w3x": "11.4c-hf9",
		`C:\Maps\Legion_TD_11.4c-hf9_Team_OZE.w3x`:        "11.4c-hf9",
	} {
		i, version := mapupdate.Match(tracked, name)
		if i != 1 || version != want {
			t.Errorf("Match(%q) = %d, %q; want 1, %q", name, i, version, want)
		}
	}
	for _, name := range []string{"Footmen Frenzy.w3x", "Legion TD x20.w3x", ""} {
		if i, version := mapupdate.Match(tracked, name); i != -1 || version != "" {
			t.Errorf("Match(%q) = %d, %q; want no match", name, i, version)
		}
	}
}

func TestNewestIsTheHighestVersionNotTheLatestChange(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	files := []dto.InstalledMap{
		// The game downloaded an old version just now, for someone's lobby.
		{Version: "11.4a", Path: "/Maps/Download/Legion_TD_11.4a_Team_OZE.w3x", Modified: now},
		{Version: "11.4c-hf9", Path: "/Maps/Download/Legion_TD_11.4c-hf9_Team_OZE.w3x", Modified: now.Add(-time.Hour)},
		{Version: "11.4c-hf10", Path: "/Maps/Download/Legion_TD_11.4c-hf10_Team_OZE.w3x", Modified: now.Add(-2 * time.Hour)},
		{Version: "11.4c", Path: "/Maps/Download/Legion_TD_11.4c_Team_OZE.w3x", Modified: now.Add(-3 * time.Hour)},
		{Version: "6.2b", Path: "/Maps/Download/Legion_TD_6.2b_Team_OZE.w3x", Modified: now.Add(-4 * time.Hour)},
	}
	got, ok := mapupdate.Newest(files)
	if !ok || got.Version != "11.4c-hf10" {
		t.Fatalf("Newest = %+v, %v; want 11.4c-hf10", got, ok)
	}
	if _, ok := mapupdate.Newest(nil); ok {
		t.Fatal("Newest found a file in an empty folder")
	}
}

func TestNewestPrefersTheLatestCopyOfTheSameVersion(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	got, _ := mapupdate.Newest([]dto.InstalledMap{
		{Version: "11.4c-hf9", Path: "/Maps/a.w3x", Modified: now.Add(-time.Hour)},
		{Version: "11.4C-HF9", Path: "/Maps/b.w3x", Modified: now},
	})
	if got.Path != "/Maps/b.w3x" {
		t.Fatalf("Newest = %+v; want the most recently changed copy", got)
	}
}

func TestCompareVersionsReadsNumbersAsNumbers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"11.4c-hf9", "11.4c-hf10", -1},
		{"11.4c", "11.4c-hf1", -1},
		{"11.4c-hf9", "11.4d", -1},
		{"6.2b", "11.2c-hf1", -1},
		{"11.4C-HF9", "11.4c-hf9", 0},
		{"11.5", "11.4c-hf9", 1},
	} {
		if got := mapupdate.CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d; want %d", c.a, c.b, got, c.want)
		}
	}
}
