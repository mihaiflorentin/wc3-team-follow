package update_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const releases = "https://github.com/mihaiflorentin/wc3-team-follow/releases"

type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }
func newClock() *clock                   { return &clock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)} }
func idle() update.Activity              { return update.Activity{} }
func release(tag string) dto.Release {
	return dto.Release{Tag: tag, Notes: "Notes for " + tag, PageURL: releases + "/tag/" + tag,
		AssetName: "wc3-team-follow-macos-arm64.zip", AssetURL: releases + "/download/" + tag + "/wc3-team-follow-macos-arm64.zip",
		ChecksumURL: releases + "/download/" + tag + "/checksums.txt"}
}

func newChecker(c *clock, version string, automatic bool, skipped string) *update.Checker {
	return update.New(update.Options{Version: version, Interval: 6 * time.Hour, RetryInterval: 30 * time.Minute, ReleasesURL: releases, Now: c.Now},
		automatic, skipped)
}

// check runs one check the way the companion does and returns the state.
func check(t *testing.T, u *update.Checker, manual bool, rel dto.Release, err error) dto.UpdateState {
	t.Helper()
	if !u.Begin(manual) {
		t.Fatal("check refused")
	}
	if got := u.State(idle()).Phase; got != dto.UpdateChecking {
		t.Fatalf("phase while checking = %q", got)
	}
	u.Finish(rel, err)
	return u.State(idle())
}

type rateLimited struct{ at time.Time }

func (e rateLimited) Error() string      { return "github rate limit" }
func (e rateLimited) RetryAt() time.Time { return e.at }

func TestAutomaticChecksRunAtStartAndThenEveryInterval(t *testing.T) {
	c := newClock()
	u := newChecker(c, "v0.7.0", true, "")
	if !u.Due() {
		t.Fatal("no check at start-up")
	}
	check(t, u, false, release("v0.7.0"), nil)
	if u.Due() {
		t.Fatal("checked again straight away")
	}
	c.advance(6*time.Hour - time.Second)
	if u.Due() {
		t.Fatal("checked before the interval")
	}
	c.advance(time.Second)
	if !u.Due() {
		t.Fatal("no check after the interval")
	}
}

func TestNoAutomaticChecksWhenTurnedOffButTheButtonStillWorks(t *testing.T) {
	c := newClock()
	u := newChecker(c, "v0.7.0", false, "")
	if u.Due() {
		t.Fatal("checked with automatic checks off")
	}
	if s := check(t, u, true, release("v0.8.0"), nil); !s.Prompt || s.Release.Version != "0.8.0" || s.CheckAutomatically {
		t.Fatalf("manual check = %+v", s)
	}
	u.SetAutomatic(true)
	c.advance(6 * time.Hour)
	if !u.Due() || !u.Automatic() {
		t.Fatal("turning checks on didn't schedule them")
	}
}

func TestOnlyOneCheckRunsAtATime(t *testing.T) {
	u := newChecker(newClock(), "v0.7.0", true, "")
	if !u.Begin(false) {
		t.Fatal("first check refused")
	}
	if u.Due() || u.Begin(true) {
		t.Fatal("a second check started while one runs")
	}
	// The player's click still gets the answer of the check that runs.
	u.Finish(dto.Release{}, errors.New("dial tcp: no route to host"))
	if s := u.State(idle()); !strings.Contains(s.Message, "couldn't check") {
		t.Fatalf("message = %q", s.Message)
	}
}

func TestFailedAutomaticChecksStayQuietAndRetryLater(t *testing.T) {
	c := newClock()
	u := newChecker(c, "v0.7.0", true, "")
	s := check(t, u, false, dto.Release{}, errors.New("dial tcp: lookup api.github.com: no such host"))
	if s.Message != "" || s.Phase != dto.UpdateIdle || s.Prompt {
		t.Fatalf("offline check wasn't quiet: %+v", s)
	}
	c.advance(29 * time.Minute)
	if u.Due() {
		t.Fatal("retried too soon")
	}
	c.advance(time.Minute)
	if !u.Due() {
		t.Fatal("no retry after an offline check")
	}

	s = check(t, u, false, dto.Release{}, rateLimited{at: c.now.Add(2 * time.Hour)})
	if s.Message != "" {
		t.Fatalf("rate-limited check wasn't quiet: %+v", s)
	}
	c.advance(time.Hour)
	if u.Due() {
		t.Fatal("retried before GitHub's rate limit reset")
	}
	c.advance(time.Hour)
	if !u.Due() {
		t.Fatal("no retry after the rate limit reset")
	}
}

func TestAFailedCheckKeepsTheReleaseAlreadyFound(t *testing.T) {
	c := newClock()
	u := newChecker(c, "v0.7.0", true, "")
	check(t, u, false, release("v0.8.0"), nil)
	c.advance(6 * time.Hour)
	if s := check(t, u, false, dto.Release{}, errors.New("timeout")); !s.Prompt || s.Release == nil {
		t.Fatalf("lost the release: %+v", s)
	}
}

func TestManualCheckFailuresSayWhy(t *testing.T) {
	u := newChecker(newClock(), "v0.7.0", true, "")
	if s := check(t, u, true, dto.Release{}, errors.New("timeout")); !strings.Contains(s.Message, "internet connection") {
		t.Fatalf("offline message = %q", s.Message)
	}
	if s := check(t, u, true, dto.Release{}, rateLimited{}); !strings.Contains(s.Message, "GitHub") {
		t.Fatalf("rate-limit message = %q", s.Message)
	}
	if s := check(t, u, true, release("v0.7.0"), nil); s.Message != "Team Follow is up to date." {
		t.Fatalf("up-to-date message = %q", s.Message)
	}
}

func TestOnlyANewerCleanReleaseWithAFileForThisSystemIsOffered(t *testing.T) {
	noAsset := release("v0.8.0")
	noAsset.AssetURL = ""
	noChecksums := release("v0.8.0")
	noChecksums.ChecksumURL = ""
	for name, rel := range map[string]dto.Release{
		"same":         release("v0.7.0"),
		"older":        release("v0.6.1"),
		"none":         {},
		"edge":         release("edge"),
		"pre-release":  release("v0.8.0-beta.1"),
		"no asset":     noAsset,
		"no checksums": noChecksums,
	} {
		u := newChecker(newClock(), "v0.7.0", true, "")
		if s := check(t, u, false, rel, nil); s.Release != nil || s.Prompt || s.Phase != dto.UpdateIdle {
			t.Errorf("%s: offered %+v", name, s)
		}
	}
	u := newChecker(newClock(), "v0.7.0", true, "")
	if s := check(t, u, true, noAsset, nil); !strings.Contains(s.Message, "0.8.0") || s.Release != nil {
		t.Fatalf("missing asset message = %+v", s)
	}
	s := check(t, u, false, release("0.10.0"), nil)
	if !s.Prompt || s.Phase != dto.UpdateAvailable || s.Release.Version != "0.10.0" || s.Release.URL != releases+"/tag/0.10.0" {
		t.Fatalf("newer release = %+v", s)
	}
}

func TestDevelopmentBuildsNeverCheck(t *testing.T) {
	for _, version := range []string{"dev", "v0.7.0-3-gabc1234-dirty", "0.0.0-edge.abc1234", "abc1234"} {
		u := newChecker(newClock(), version, true, "")
		s := u.State(idle())
		if u.Due() || u.Begin(true) || s.Supported || s.CheckAutomatically && s.Supported {
			t.Errorf("%s: checks (state %+v)", version, s)
		}
		if s.CurrentVersion != version || !strings.Contains(s.Message, "development") {
			t.Errorf("%s: state = %+v", version, s)
		}
	}
	s := newChecker(newClock(), "v0.7.0", true, "").State(idle())
	if !s.Supported || s.CurrentVersion != "0.7.0" || s.ReleasesURL != releases || s.Message != "" {
		t.Fatalf("release build = %+v", s)
	}
}

func TestLaterHidesTheBannerUntilANewerReleaseOrAManualCheck(t *testing.T) {
	c := newClock()
	u := newChecker(c, "v0.7.0", true, "")
	check(t, u, false, release("v0.8.0"), nil)
	u.Later()
	s := u.State(idle())
	if s.Prompt || s.Release == nil || s.Phase != dto.UpdateAvailable {
		t.Fatalf("after Later = %+v", s)
	}
	c.advance(6 * time.Hour)
	if s := check(t, u, false, release("v0.8.0"), nil); s.Prompt {
		t.Fatal("the automatic check brought the same version back")
	}
	if s := check(t, u, false, release("v0.8.1"), nil); !s.Prompt {
		t.Fatal("a newer release stayed hidden")
	}
	u.Later()
	if s := check(t, u, true, release("v0.8.1"), nil); !s.Prompt {
		t.Fatal("Check for updates didn't show the banner again")
	}
}

func TestSkipSilencesThatVersionButNotTheNext(t *testing.T) {
	c := newClock()
	u := newChecker(c, "v0.7.0", true, "")
	if _, err := u.Skip(); err == nil {
		t.Fatal("skipped with nothing offered")
	}
	check(t, u, false, release("v0.8.0"), nil)
	skipped, err := u.Skip()
	if err != nil || skipped != "0.8.0" || u.SkippedVersion() != "0.8.0" {
		t.Fatalf("Skip = %q, %v", skipped, err)
	}
	if s := u.State(idle()); s.Prompt || !s.Skipped {
		t.Fatalf("after Skip = %+v", s)
	}
	if s := check(t, u, true, release("v0.8.0"), nil); s.Prompt || !s.Skipped {
		t.Fatalf("Check for updates brought a skipped version back: %+v", s)
	}
	if s := check(t, u, false, release("v0.9.0"), nil); !s.Prompt || s.Skipped {
		t.Fatalf("the next version stayed silent: %+v", s)
	}

	// A skipped version saved earlier stays silent after a restart.
	u = newChecker(newClock(), "v0.7.0", true, "0.9.0")
	if s := check(t, u, false, release("v0.9.0"), nil); s.Prompt || !s.Skipped {
		t.Fatalf("saved skip ignored: %+v", s)
	}
}

func TestWarningWhenUpdatingWouldInterruptPlay(t *testing.T) {
	u := newChecker(newClock(), "v0.7.0", true, "")
	check(t, u, false, release("v0.8.0"), nil)
	for _, tc := range []struct {
		activity update.Activity
		want     string
	}{
		{update.Activity{}, ""},
		{update.Activity{InLobby: true}, "You're in a lobby or a match"},
		{update.Activity{InMatch: true, AutoJoin: true}, "You're in a lobby or a match"},
		{update.Activity{AutoJoin: true}, "Auto-join is on"},
		{update.Activity{AutoHost: true}, "Auto-host is on"},
		{update.Activity{AutoJoin: true, AutoHost: true}, "Auto-join and Auto-host are on"},
	} {
		s := u.State(tc.activity)
		if tc.want == "" && s.Warning != "" || !strings.HasPrefix(s.Warning, tc.want) {
			t.Errorf("%+v: warning = %q", tc.activity, s.Warning)
		}
		if tc.want != "" && !strings.Contains(s.Warning, "restarts Team Follow") {
			t.Errorf("%+v: warning = %q", tc.activity, s.Warning)
		}
	}
	// Nothing to warn about without a release.
	if s := newChecker(newClock(), "v0.7.0", true, "").State(update.Activity{InMatch: true}); s.Warning != "" {
		t.Fatalf("warning without a release: %q", s.Warning)
	}
}

func TestTheOfferCarriesTheChangelogSinceTheRunningVersion(t *testing.T) {
	u := newChecker(newClock(), "v0.7.0", true, "")
	rel := release("v0.8.0")
	rel.Changelog = sampleChangelog
	rel.Notes = "## What's Changed\n* <b>Raw</b> notes by @someone"
	s := check(t, u, false, rel, nil)
	if got := s.Release.Changes; len(got) != 2 || got[0].Version != "0.8.0" || got[1].Version != "0.7.1" {
		t.Fatalf("changes = %+v", got)
	}

	// Without a usable changelog the release notes are the fallback.
	for name, changelog := range map[string]string{"missing": "", "no matching section": "## [0.6.0]\n### Added\n- Old.\n"} {
		rel.Changelog = changelog
		s = check(t, newChecker(newClock(), "v0.7.0", true, ""), false, rel, nil)
		if len(s.Release.Changes) != 0 || s.Release.Changes == nil || s.Release.Notes != "What's Changed\n• <b>Raw</b> notes by @someone" {
			t.Errorf("%s: release = %+v", name, s.Release)
		}
	}
}

func TestLastCheckedIsTheTimeOfTheLastFinishedCheck(t *testing.T) {
	c := newClock()
	u := newChecker(c, "v0.7.0", true, "")
	if u.State(idle()).LastChecked != nil {
		t.Fatal("checked before any check")
	}
	check(t, u, false, release("v0.7.0"), nil)
	if got := u.State(idle()).LastChecked; got == nil || !got.Equal(c.now) {
		t.Fatalf("last checked = %v", got)
	}
}
