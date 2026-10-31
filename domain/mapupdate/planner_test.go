package mapupdate_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/mapupdate"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var legionTD = dto.TrackedMap{Name: "Legion TD", Repository: "team-oze/Legion-TD", Folder: "Download"}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }
func newClock() *clock                   { return &clock{now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)} }
func installed(versions ...string) []dto.InstalledMap {
	var out []dto.InstalledMap
	for _, v := range versions {
		out = append(out, dto.InstalledMap{Version: v, Path: "/Maps/Download/Legion_TD_" + v + "_Team_OZE.w3x"})
	}
	return out
}

var hf9 = dto.MapRelease{Map: legionTD, Tag: "v11.4c-hf9", Version: "11.4c-hf9", AssetName: "Legion_TD_11.4c-hf9_Team_OZE.w3x",
	Size: 31_657_000, PageURL: "https://github.com/team-oze/Legion-TD/releases/tag/v11.4c-hf9"}

func newPlanner(c *clock, enabled bool) *mapupdate.Planner {
	return mapupdate.New(mapupdate.Options{Interval: 6 * time.Hour, RetryInterval: 30 * time.Minute, Now: c.Now},
		[]dto.TrackedMap{legionTD}, enabled)
}

type rateLimit struct{ at time.Time }

func (r rateLimit) Error() string      { return "rate limited" }
func (r rateLimit) RetryAt() time.Time { return r.at }

type problem string

func (p problem) Error() string   { return string(p) }
func (p problem) Problem() string { return string(p) }

func TestNewestMissingIsDownloadedAndReportedReady(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	if !p.Due() || !p.Begin(false) {
		t.Fatal("first check isn't due at start")
	}
	if row := p.State().Maps[0]; row.Phase != dto.MapChecking || row.Name != "Legion TD" {
		t.Fatalf("row = %+v", row)
	}
	if !p.Found(0, hf9, installed("11.4c-hf5", "11.2c-hf1"), nil) {
		t.Fatal("the missing newest version isn't downloaded")
	}
	row := p.State().Maps[0]
	if row.Phase != dto.MapDownloading || row.Latest != "11.4c-hf9" || row.Installed != "11.4c-hf5" || row.Progress == nil {
		t.Fatalf("row = %+v", row)
	}
	if notes := p.TakeNotes(); len(notes) != 1 || notes[0] != "Downloading Legion TD 11.4c-hf9 into your Maps folder." {
		t.Fatalf("notes = %q", notes)
	}
	p.Progress(0, dto.MapProgress{Done: 12 << 20, Total: 30 << 20})
	if pr := p.State().Maps[0].Progress; pr.Done != 12<<20 || pr.Total != 30<<20 {
		t.Fatalf("progress = %+v", pr)
	}
	p.Downloaded(0, "/Maps/Download/Legion_TD_11.4c-hf9_Team_OZE.w3x", nil)
	p.Finish()
	row = p.State().Maps[0]
	if row.Phase != dto.MapDone || row.Installed != "11.4c-hf9" || row.Progress != nil || row.Message != "Legion TD 11.4c-hf9 is ready in your Maps folder." {
		t.Fatalf("row = %+v", row)
	}
	if notes := p.TakeNotes(); len(notes) != 1 || notes[0] != "Legion TD 11.4c-hf9 is ready in your Maps folder." {
		t.Fatalf("notes = %q", notes)
	}
	if s := p.State(); s.Checking || s.LastChecked == nil {
		t.Fatalf("state = %+v", s)
	}
}

func TestInstalledNewestIsNotFetchedAgain(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(false)
	// Versions are compared as strings, ignoring case.
	if p.Found(0, hf9, installed("11.4C-HF9"), nil) {
		t.Fatal("downloads a version that is installed")
	}
	p.Finish()
	row := p.State().Maps[0]
	if row.Phase != dto.MapUpToDate || row.Installed != "11.4C-HF9" || row.Latest != "11.4c-hf9" || row.PageURL != hf9.PageURL {
		t.Fatalf("row = %+v", row)
	}
	if notes := p.TakeNotes(); len(notes) != 0 {
		t.Fatalf("notes = %q", notes)
	}
}

func TestSwitchedOffNothingIsCheckedOrDownloaded(t *testing.T) {
	c := newClock()
	p := newPlanner(c, false)
	if p.Due() || p.State().Enabled {
		t.Fatal("checks with Keep maps up to date off")
	}
	// Check now still answers, but never downloads.
	if !p.Begin(true) {
		t.Fatal("Check now refused")
	}
	if p.Found(0, hf9, installed("11.4c-hf5"), nil) {
		t.Fatal("downloads with the switch off")
	}
	p.Finish()
	row := p.State().Maps[0]
	if row.Phase != dto.MapMissing || row.Message != "Legion TD 11.4c-hf9 is out. Turn on Keep maps up to date to download it." {
		t.Fatalf("row = %+v", row)
	}
	// Switching on checks at once.
	p.SetEnabled(true)
	if !p.Due() {
		t.Fatal("switching on doesn't check")
	}
}

func TestChecksRepeatEveryIntervalAndOneAtATime(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(false)
	if p.Begin(true) || p.Due() {
		t.Fatal("a second check started while one runs")
	}
	p.Found(0, hf9, installed("11.4c-hf9"), nil)
	p.Finish()
	c.advance(6*time.Hour - time.Minute)
	if p.Due() {
		t.Fatal("due before the interval")
	}
	c.advance(time.Minute)
	if !p.Due() {
		t.Fatal("not due after the interval")
	}
}

func TestOfflineChecksAreQuietAndRetried(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(false)
	p.Found(0, hf9, installed("11.4c-hf9"), nil)
	p.Finish()
	c.advance(6 * time.Hour)
	p.Begin(false)
	if p.Found(0, dto.MapRelease{}, nil, errors.New("dial tcp: no such host")) {
		t.Fatal("downloads after a failed check")
	}
	p.Finish()
	// What was found before stays; nothing is said.
	row := p.State().Maps[0]
	if row.Phase != dto.MapUpToDate || row.Latest != "11.4c-hf9" || row.Message != "" || p.State().Message != "" {
		t.Fatalf("row = %+v", row)
	}
	if notes := p.TakeNotes(); len(notes) != 0 {
		t.Fatalf("notes = %q", notes)
	}
	c.advance(29 * time.Minute)
	if p.Due() {
		t.Fatal("retried too early")
	}
	c.advance(time.Minute)
	if !p.Due() {
		t.Fatal("not retried")
	}
}

func TestRateLimitWaitsUntilGitHubAllowsAgain(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(false)
	p.Found(0, dto.MapRelease{}, nil, fmt.Errorf("read: %w", rateLimit{at: c.now.Add(2 * time.Hour)}))
	p.Finish()
	c.advance(time.Hour)
	if p.Due() {
		t.Fatal("checked during the rate limit")
	}
	c.advance(time.Hour)
	if !p.Due() {
		t.Fatal("not checked after the rate limit")
	}
}

func TestCheckNowSaysWhyItFailed(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(true)
	p.Found(0, dto.MapRelease{}, nil, errors.New("dial tcp: no such host"))
	p.Finish()
	row := p.State().Maps[0]
	if row.Phase != dto.MapFailed || row.Message != "Team Follow couldn't reach GitHub to check Legion TD. Check your internet connection and try again." {
		t.Fatalf("row = %+v", row)
	}
	p.Begin(true)
	p.Abort("No Warcraft III maps folder was found. Choose it yourself.")
	if s := p.State(); s.Checking || s.Message != "No Warcraft III maps folder was found. Choose it yourself." {
		t.Fatalf("state = %+v", s)
	}
}

func TestARefusedReleaseIsShownEvenWhenAutomatic(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(false)
	p.Found(0, dto.MapRelease{}, nil, problem("Legion TD 11.4c-hf9 has no published checksum on GitHub, so it isn't downloaded."))
	p.Finish()
	if row := p.State().Maps[0]; row.Phase != dto.MapFailed || row.Message != "Legion TD 11.4c-hf9 has no published checksum on GitHub, so it isn't downloaded." {
		t.Fatalf("row = %+v", row)
	}
}

func TestAFailedDownloadSaysWhyAndIsRetried(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(false)
	p.Found(0, hf9, nil, nil)
	_ = p.TakeNotes()
	p.Downloaded(0, "", problem("The download of Legion TD 11.4c-hf9 didn't match its published checksum, so it wasn't kept."))
	p.Finish()
	row := p.State().Maps[0]
	want := "Legion TD 11.4c-hf9 couldn't be downloaded. The download of Legion TD 11.4c-hf9 didn't match its published checksum, so it wasn't kept."
	if row.Phase != dto.MapFailed || row.Message != want || row.Progress != nil || row.Installed != "" {
		t.Fatalf("row = %+v", row)
	}
	if notes := p.TakeNotes(); len(notes) != 1 || notes[0] != want {
		t.Fatalf("notes = %q", notes)
	}
	p.Begin(false)
	p.Downloaded(0, "", errors.New("download: connection reset"))
	if row := p.State().Maps[0]; row.Message != "Legion TD 11.4c-hf9 couldn't be downloaded. Check your internet connection and try again." {
		t.Fatalf("row = %+v", row)
	}
	p.Finish()
	c.advance(30 * time.Minute)
	if !p.Due() {
		t.Fatal("a failed download isn't retried")
	}
}

func TestSwitchingOffStopsTheDownload(t *testing.T) {
	c := newClock()
	p := newPlanner(c, true)
	p.Begin(false)
	p.Found(0, hf9, installed("11.4c-hf5"), nil)
	p.SetEnabled(false)
	p.Cancelled(0)
	p.Finish()
	row := p.State().Maps[0]
	if row.Phase != dto.MapMissing || row.Progress != nil || p.Due() {
		t.Fatalf("row = %+v", row)
	}
}

func TestNoTrackedMapsNeverChecks(t *testing.T) {
	p := mapupdate.New(mapupdate.Options{Now: newClock().Now}, nil, true)
	if p.Due() || p.Begin(true) {
		t.Fatal("checks without tracked maps")
	}
	if s := p.State(); s.Maps == nil || len(s.Maps) != 0 {
		t.Fatalf("maps = %v", s.Maps)
	}
}
