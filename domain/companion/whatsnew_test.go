package companion_test

import (
	"errors"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const builtInChangelog = `# Changelog

## [Unreleased]

### Added

- Not released yet.

## [0.8.0] - 2026-11-02

### Added

- **Update check.** Sample entry.

## [0.7.1] - 2026-10-30

### Fixed

- A fix.

## [0.7.0] - 2026-10-28

### Added

- **Recent players.**
`

func whatsNewPrefs(last string, stored bool) *mockprefs.Store {
	return mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: true, AutoBan: true, BlacklistManualBans: true,
		CheckUpdates: false, LastVersion: last, Stored: stored})
}

func newWhatsNewHarness(t *testing.T, prefs *mockprefs.Store, version string) *harness {
	t.Helper()
	return newHarnessWith(t, prefs, mockrecent.New(), func(o *companion.Options) {
		o.AppVersion = version
		o.Changelog = builtInChangelog
	})
}

func TestWhatsNewShowsOnTheFirstStartAfterAnUpdateAndSavesTheVersion(t *testing.T) {
	prefs := whatsNewPrefs("0.7.0", true)
	h := newWhatsNewHarness(t, prefs, "v0.8.0")
	w := h.svc.State().Update.WhatsNew
	if w == nil || !w.Show || w.Version != "0.8.0" || w.Since != "0.7.0" || len(w.Changes) != 2 {
		t.Fatalf("whats new = %+v", w)
	}
	if got, _ := prefs.Load(); got.LastVersion != "0.8.0" || got.Tags == nil {
		t.Fatalf("saved %+v", got)
	}

	// The next start shows nothing.
	again := newWhatsNewHarness(t, prefs, "v0.8.0")
	if w := again.svc.State().Update.WhatsNew; w == nil || w.Show {
		t.Fatalf("second start = %+v", w)
	}
}

func TestDismissWhatsNewClosesIt(t *testing.T) {
	h := newWhatsNewHarness(t, whatsNewPrefs("0.7.0", true), "v0.8.0")
	s := h.svc.DismissWhatsNew()
	if w := s.Update.WhatsNew; w == nil || w.Show || len(w.Changes) != 2 {
		t.Fatalf("dismissed = %+v", w)
	}
	if w := h.svc.State().Update.WhatsNew; w.Show {
		t.Fatal("shown again")
	}
}

func TestWhatsNewOnAFreshInstallShowsNothing(t *testing.T) {
	prefs := whatsNewPrefs("", false)
	h := newWhatsNewHarness(t, prefs, "v0.8.0")
	if w := h.svc.State().Update.WhatsNew; w == nil || w.Show {
		t.Fatalf("fresh install = %+v", w)
	}
	if got, _ := prefs.Load(); got.LastVersion != "0.8.0" {
		t.Fatalf("saved %+v", got)
	}
}

func TestWhatsNewOnAnOlderInstallShowsThisVersionOnce(t *testing.T) {
	h := newWhatsNewHarness(t, whatsNewPrefs("", true), "v0.8.0")
	if w := h.svc.State().Update.WhatsNew; w == nil || !w.Show || len(w.Changes) != 1 || w.Changes[0].Version != "0.8.0" {
		t.Fatalf("older install = %+v", w)
	}
}

func TestWhatsNewStaysQuietWhenSettingsCannotBeLoaded(t *testing.T) {
	prefs := whatsNewPrefs("0.7.0", true)
	prefs.LoadErr = errors.New("unreadable")
	h := newWhatsNewHarness(t, prefs, "v0.8.0")
	if w := h.svc.State().Update.WhatsNew; w != nil && w.Show {
		t.Fatalf("whats new = %+v", w)
	}
	if prefs.Saves != 0 {
		t.Fatalf("saved %d times over an unreadable file", prefs.Saves)
	}
}

func TestChangelogListsEveryReleasedVersionNewestFirst(t *testing.T) {
	h := newWhatsNewHarness(t, whatsNewPrefs("0.8.0", true), "v0.8.0")
	got := h.svc.Changelog()
	if len(got) != 3 || got[0].Version != "0.8.0" || got[2].Version != "0.7.0" || got[0].Date != "2026-11-02" {
		t.Fatalf("changelog = %+v", got)
	}
}
