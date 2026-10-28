package companion_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const releasePage = "https://github.com/mihaiflorentin/wc3-team-follow/releases"

func newerRelease(tag string) dto.Release {
	return dto.Release{Tag: tag, Notes: "Notes", PageURL: releasePage + "/tag/" + tag,
		AssetName: "wc3-team-follow-linux-amd64.tar.gz", AssetURL: releasePage + "/download/" + tag + "/wc3-team-follow-linux-amd64.tar.gz",
		ChecksumURL: releasePage + "/download/" + tag + "/checksums.txt",
		Changelog:   "## [0.8.0] - 2026-11-02\n### Added\n- **Update check.** Sample entry.\n"}
}

func updatePrefs(check bool, skipped string) *mockprefs.Store {
	return mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: true, AutoBan: true, BlacklistManualBans: true,
		CheckUpdates: check, SkippedVersion: skipped})
}

func newUpdateHarness(t *testing.T, prefs *mockprefs.Store, version string) *harness {
	t.Helper()
	return newHarnessWith(t, prefs, mockrecent.New(), func(o *companion.Options) {
		o.AppVersion = version
		o.UpdateInterval = 6 * time.Hour
		o.UpdateRetryInterval = 30 * time.Minute
		o.ReleasesURL = releasePage
	})
}

func TestCompanionChecksAtStartAndOffersTheNewerRelease(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(true, ""), "v0.7.0")
	h.updater.Set(newerRelease("v0.8.0"), nil)
	h.run(t)
	waitFor(t, "banner", func() bool { return h.svc.State().Update.Prompt })
	u := h.svc.State().Update
	if u.Release.Version != "0.8.0" || u.CurrentVersion != "0.7.0" || len(u.Release.Changes) != 1 || u.ReleasesURL != releasePage {
		t.Fatalf("update = %+v", u)
	}
	time.Sleep(1200 * time.Millisecond)
	if n := h.updater.Calls(); n != 1 {
		t.Fatalf("checked %d times at start", n)
	}
	h.skew.Store(int64(6 * time.Hour))
	waitFor(t, "second check", func() bool { return h.updater.Calls() == 2 })
}

func TestCompanionStaysQuietWhenTheCheckFails(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(true, ""), "v0.7.0")
	h.updater.Set(dto.Release{}, errors.New("dial tcp: no such host"))
	h.run(t)
	waitFor(t, "check", func() bool { return h.updater.Calls() == 1 })
	waitFor(t, "check finished", func() bool { return h.svc.State().Update.Phase == dto.UpdateIdle })
	if u := h.svc.State().Update; u.Message != "" || u.Prompt {
		t.Fatalf("update = %+v", u)
	}
}

func TestCheckAutomaticallyIsSavedAndStopsChecks(t *testing.T) {
	prefs := updatePrefs(false, "")
	h := newUpdateHarness(t, prefs, "v0.7.0")
	h.run(t)
	time.Sleep(1200 * time.Millisecond)
	if h.updater.Calls() != 0 || h.svc.State().Update.CheckAutomatically {
		t.Fatal("checked with automatic checks off")
	}
	on := true
	s, err := h.svc.Configure(dto.FollowConfig{CheckUpdates: &on})
	if err != nil || !s.Update.CheckAutomatically {
		t.Fatalf("Configure = %+v, %v", s.Update, err)
	}
	if got, _ := prefs.Load(); !got.CheckUpdates {
		t.Fatalf("not saved: %+v", got)
	}
	waitFor(t, "check after turning it on", func() bool { return h.updater.Calls() == 1 })
}

func TestCheckForUpdatesAnswersThePlayer(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(false, ""), "v0.7.0")
	h.updater.Set(dto.Release{Tag: "v0.7.0"}, nil)
	hold := make(chan struct{})
	h.updater.Hold = hold
	h.run(t)
	waitFor(t, "running", func() bool { return h.svc.State().Connected })
	s, err := h.svc.CheckForUpdates()
	if err != nil || s.Update.Phase != dto.UpdateChecking {
		t.Fatalf("CheckForUpdates = %+v, %v", s.Update, err)
	}
	close(hold)
	waitFor(t, "answer", func() bool { return h.svc.State().Update.Message == "Team Follow is up to date." })
}

func TestSkipIsSavedAndLaterIsNot(t *testing.T) {
	prefs := updatePrefs(true, "")
	h := newUpdateHarness(t, prefs, "v0.7.0")
	h.updater.Set(newerRelease("v0.8.0"), nil)
	h.run(t)
	waitFor(t, "banner", func() bool { return h.svc.State().Update.Prompt })

	if s := h.svc.UpdateLater(); s.Update.Prompt || s.Update.Release == nil {
		t.Fatalf("Later = %+v", s.Update)
	}
	if got, _ := prefs.Load(); got.SkippedVersion != "" || prefs.Saves != 0 {
		t.Fatalf("Later saved %+v", got)
	}
	s, err := h.svc.SkipUpdate()
	if err != nil || !s.Update.Skipped || s.Update.Prompt {
		t.Fatalf("Skip = %+v, %v", s.Update, err)
	}
	if got, _ := prefs.Load(); got.SkippedVersion != "0.8.0" || !got.CheckUpdates {
		t.Fatalf("saved %+v", got)
	}
}

func TestASavedSkipKeepsThatVersionQuiet(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(true, "0.8.0"), "v0.7.0")
	h.updater.Set(newerRelease("v0.8.0"), nil)
	h.run(t)
	waitFor(t, "check", func() bool { return h.svc.State().Update.Release != nil })
	if u := h.svc.State().Update; u.Prompt || !u.Skipped {
		t.Fatalf("update = %+v", u)
	}
}

func TestTheBannerWarnsDuringAMatch(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(true, ""), "v0.7.0")
	h.updater.Set(newerRelease("v0.8.0"), nil)
	h.run(t)
	waitFor(t, "banner", func() bool { return h.svc.State().Update.Prompt })
	if w := h.svc.State().Update.Warning; w != "" {
		t.Fatalf("warning in the menus: %q", w)
	}
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	h.session.Push(dto.GameEvent{MessageType: "IsGameUIActive", Payload: map[string]any{"isActive": true}})
	waitFor(t, "warning", func() bool { return strings.HasPrefix(h.svc.State().Update.Warning, "You're in a lobby or a match.") })
}

func TestDevelopmentBuildsNeverAskGitHub(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(true, ""), "v0.7.0-3-gabc1234-dirty")
	h.updater.Set(newerRelease("v0.8.0"), nil)
	h.run(t)
	if _, err := h.svc.CheckForUpdates(); err == nil {
		t.Fatal("a development build checked for updates")
	}
	time.Sleep(1200 * time.Millisecond)
	if h.updater.Calls() != 0 {
		t.Fatal("a development build asked GitHub")
	}
	if u := h.svc.State().Update; u.Supported || !strings.Contains(u.Message, "development") {
		t.Fatalf("update = %+v", u)
	}
}
