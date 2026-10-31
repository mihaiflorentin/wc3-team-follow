package maps_test

import (
	"context"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/maps"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// legionTD is the tracked map as config.toml describes it.
var legionTD = dto.TrackedMap{Name: "Legion TD", Repository: "team-oze/Legion-TD",
	Pattern: `(?i)^Legion_TD_(?P<version>\d[^_/\\ ]*)_Team_?OZE(?: \(\d+\))?\.w3x$`, Folder: "Download"}

func TestInstalledFindsEveryNameVariantNewestFirst(t *testing.T) {
	root := t.TempDir()
	// Oldest first: touch dates them a day apart.
	touch(t, root,
		"Download/Legion_TD_6.2b_Team_OZE (1).w3x",
		"Download/Legion_TD_11.2c-hf1_TeamOZE.w3x",
		"Download/Legion TD x20.w3x",
		"Download/Legion_TD_11.4c-hf5_Team_OZE.w3x.part",
		"Download/.wc3-team-follow-map-123.part",
		"Legion_TD_Mega_Team_OZE.w3x",
		"Download/0123456789abcdef0123456789abcdef01234567/Legion_TD_11.3a_Team_OZE.w3x",
		"Download/Legion_TD_11.4c-hf5_Team_OZE.w3x",
	)
	got, err := maps.New().Installed(context.Background(), root, legionTD)
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, m := range got {
		versions = append(versions, m.Version)
	}
	want := []string{"11.4c-hf5", "11.3a", "11.2c-hf1", "6.2b"}
	if len(versions) != len(want) {
		t.Fatalf("versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("versions = %v, want %v", versions, want)
		}
	}
	if got[0].Path == "" || got[0].Modified.IsZero() {
		t.Fatalf("newest = %+v", got[0])
	}
}

func TestInstalledWithoutMatchesIsEmpty(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "Download/Footmen Frenzy.w3x")
	got, err := maps.New().Installed(context.Background(), root, legionTD)
	if err != nil || len(got) != 0 {
		t.Fatalf("Installed = %v, %v", got, err)
	}
}

func TestInstalledRefusesAPatternWithoutAVersionGroup(t *testing.T) {
	bad := legionTD
	bad.Pattern = `^Legion_TD_.*\.w3x$`
	if _, err := maps.New().Installed(context.Background(), t.TempDir(), bad); err == nil {
		t.Fatal("no error for a pattern without a version group")
	}
}

func TestMatchVersion(t *testing.T) {
	for name, want := range map[string]string{
		"Legion_TD_11.4c-hf9_Team_OZE.w3x":  "11.4c-hf9",
		"Legion_TD_11.2c-hf1_TeamOZE.w3x":   "11.2c-hf1",
		"Legion_TD_6.2b_Team_OZE (1).w3x":   "6.2b",
		"Legion_TD_11.4c_Team_OZE.w3x.part": "",
		"Legion TD 11.4c Team OZE.w3x":      "",
		"../Legion_TD_11.4c_Team_OZE.w3x":   "",
		"dir/Legion_TD_11.4c_Team_OZE.w3x":  "",
	} {
		got, err := maps.MatchVersion(legionTD.Pattern, name)
		if err != nil || got != want {
			t.Errorf("MatchVersion(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
}
