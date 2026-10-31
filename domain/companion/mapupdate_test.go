package companion_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var trackedLegionTD = dto.TrackedMap{Name: "Legion TD", Repository: "team-oze/Legion-TD", Pattern: `^Legion_TD_(?P<version>[^_]+)_Team_?OZE\.w3x$`,
	Folder: "Download"}

var legionHF9 = dto.MapRelease{Tag: "v11.4c-hf9", Version: "11.4c-hf9", AssetName: "Legion_TD_11.4c-hf9_Team_OZE.w3x", Size: 30 << 20,
	SHA256: strings.Repeat("ab", 32), URL: "https://github.com/team-oze/Legion-TD/releases/download/v11.4c-hf9/Legion_TD_11.4c-hf9_Team_OZE.w3x",
	PageURL: "https://github.com/team-oze/Legion-TD/releases/tag/v11.4c-hf9"}

func mapPrefs(on bool) *mockprefs.Store {
	return mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: true, AutoBan: true, BlacklistManualBans: true,
		MapUpdates: on})
}

func newMapHarness(t *testing.T, prefs *mockprefs.Store) *harness {
	t.Helper()
	h := newHarnessWith(t, prefs, mockrecent.New(), func(o *companion.Options) {
		o.TrackedMaps = []dto.TrackedMap{trackedLegionTD}
	})
	h.library.SetInstalled(dto.InstalledMap{Version: "11.4c-hf5", Path: "/detected/Maps/Download/Legion_TD_11.4c-hf5_Team_OZE.w3x"})
	h.releases.Set(legionHF9, nil)
	h.releases.OnDownload = func(path string) {
		h.library.SetInstalled(dto.InstalledMap{Version: "11.4c-hf9", Path: path},
			dto.InstalledMap{Version: "11.4c-hf5", Path: "/detected/Maps/Download/Legion_TD_11.4c-hf5_Team_OZE.w3x"})
	}
	return h
}

func logged(s dto.FollowState, line string) bool {
	return slices.ContainsFunc(s.Log, func(e dto.LogEntry) bool { return e.Message == line })
}

func TestMissingNewestMapIsDownloadedIntoTheMapsFolderAtStart(t *testing.T) {
	h := newMapHarness(t, mapPrefs(true))
	hold := make(chan struct{})
	h.releases.Script([]dto.MapProgress{{Done: 12 << 20, Total: 30 << 20}}, hold, nil)
	h.run(t)
	waitFor(t, "download", func() bool { return h.svc.State().MapUpdates.Maps[0].Phase == dto.MapDownloading })
	row := h.svc.State().MapUpdates.Maps[0]
	if row.Progress == nil || row.Progress.Done != 12<<20 || row.Latest != "11.4c-hf9" || row.Installed != "11.4c-hf5" {
		t.Fatalf("row = %+v", row)
	}
	if !logged(h.svc.State(), "Downloading Legion TD 11.4c-hf9 into your Maps folder.") {
		t.Fatal("no Activity note for the download")
	}
	close(hold)
	waitFor(t, "done", func() bool { return h.svc.State().MapUpdates.Maps[0].Phase == dto.MapDone })
	s := h.svc.State()
	if row := s.MapUpdates.Maps[0]; row.Message != "Legion TD 11.4c-hf9 is ready in your Maps folder." || row.Installed != "11.4c-hf9" {
		t.Fatalf("row = %+v", row)
	}
	if !logged(s, "Legion TD 11.4c-hf9 is ready in your Maps folder.") {
		t.Fatal("no Activity note for the finished download")
	}
	if dirs := h.releases.Dirs(); len(dirs) != 1 || dirs[0] != filepath.Join("/detected/Maps", "Download") {
		t.Fatalf("downloaded into %v", dirs)
	}
	// The next check finds it installed and fetches nothing.
	h.skew.Store(int64(6 * time.Hour))
	waitFor(t, "second check", func() bool { return h.releases.LatestCalls() == 2 && !h.svc.State().MapUpdates.Checking })
	if got := h.releases.Downloaded(); len(got) != 1 {
		t.Fatalf("downloaded %v", got)
	}
	if row := h.svc.State().MapUpdates.Maps[0]; row.Phase != dto.MapUpToDate {
		t.Fatalf("row = %+v", row)
	}
}

func TestKeepMapsUpToDateOffIsSavedAndNothingDownloads(t *testing.T) {
	prefs := mapPrefs(false)
	h := newMapHarness(t, prefs)
	h.run(t)
	time.Sleep(1200 * time.Millisecond)
	if h.releases.LatestCalls() != 0 || h.svc.State().MapUpdates.Enabled {
		t.Fatal("checked with Keep maps up to date off")
	}
	// Check now answers without downloading.
	if _, err := h.svc.CheckMaps(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "check", func() bool { return h.svc.State().MapUpdates.Maps[0].Phase == dto.MapMissing })
	if len(h.releases.Downloaded()) != 0 {
		t.Fatal("downloaded with the switch off")
	}
	on := true
	s, err := h.svc.Configure(dto.FollowConfig{MapUpdates: &on})
	if err != nil || !s.MapUpdates.Enabled {
		t.Fatalf("Configure = %+v, %v", s.MapUpdates, err)
	}
	if saved, _ := prefs.Load(); !saved.MapUpdates {
		t.Fatal("the switch wasn't saved")
	}
	waitFor(t, "done", func() bool { return h.svc.State().MapUpdates.Maps[0].Phase == dto.MapDone })
}

func TestSwitchingOffStopsAMapDownload(t *testing.T) {
	h := newMapHarness(t, mapPrefs(true))
	h.releases.Script(nil, make(chan struct{}), nil)
	h.run(t)
	waitFor(t, "download", func() bool { return h.svc.State().MapUpdates.Maps[0].Phase == dto.MapDownloading })
	off := false
	if _, err := h.svc.Configure(dto.FollowConfig{MapUpdates: &off}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "stopped", func() bool {
		u := h.svc.State().MapUpdates
		return !u.Checking && u.Maps[0].Phase == dto.MapMissing
	})
}

func TestAFailedMapDownloadSaysWhy(t *testing.T) {
	h := newMapHarness(t, mapPrefs(true))
	h.releases.Script(nil, nil, errors.New("download: connection reset"))
	h.run(t)
	waitFor(t, "failed", func() bool { return h.svc.State().MapUpdates.Maps[0].Phase == dto.MapFailed })
	want := "Legion TD 11.4c-hf9 couldn't be downloaded. Check your internet connection and try again."
	s := h.svc.State()
	if s.MapUpdates.Maps[0].Message != want || !logged(s, want) {
		t.Fatalf("row = %+v", s.MapUpdates.Maps[0])
	}
}

func TestCheckMapsWithoutAMapsFolderSaysSo(t *testing.T) {
	h := newMapHarness(t, mapPrefs(true))
	h.locator.Folders = nil
	h.run(t)
	waitFor(t, "Check now", func() bool { _, err := h.svc.CheckMaps(); return err == nil })
	waitFor(t, "message", func() bool { return h.svc.State().MapUpdates.Message != "" })
	if m := h.svc.State().MapUpdates.Message; !strings.Contains(m, "maps folder") {
		t.Fatalf("message = %q", m)
	}
}
