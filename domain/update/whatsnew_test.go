package update_test

import (
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
)

func versions(w *update.WhatsNew) []string {
	view := w.View()
	if view == nil {
		return nil
	}
	out := []string{}
	for _, s := range view.Changes {
		out = append(out, s.Version)
	}
	return out
}

func TestWhatsNewOnTheFirstStartAfterAnUpdateShowsEverySectionSinceTheLastVersion(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.8.0", "0.7.0", true)
	view := w.View()
	if view == nil || !view.Show || view.Version != "0.8.0" || view.Since != "0.7.0" {
		t.Fatalf("view = %+v", view)
	}
	if got := versions(w); len(got) != 2 || got[0] != "0.8.0" || got[1] != "0.7.1" {
		t.Fatalf("versions = %v", got)
	}
	if !w.NeedsSave() || w.LastVersion() != "0.8.0" {
		t.Fatalf("save = %v %q", w.NeedsSave(), w.LastVersion())
	}
}

func TestWhatsNewIsNotShownAgainOnTheSecondStart(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.8.0", "0.8.0", true)
	if view := w.View(); view == nil || view.Show {
		t.Fatalf("view = %+v", view)
	}
	// The link still reopens this version's entries.
	if got := versions(w); len(got) != 1 || got[0] != "0.8.0" {
		t.Fatalf("versions = %v", got)
	}
	if w.NeedsSave() {
		t.Fatal("saved again")
	}
}

func TestWhatsNewOnAFreshInstallShowsNothingButRemembersTheVersion(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.8.0", "", false)
	if view := w.View(); view == nil || view.Show || view.Since != "" {
		t.Fatalf("view = %+v", view)
	}
	if !w.NeedsSave() || w.LastVersion() != "0.8.0" {
		t.Fatalf("save = %v %q", w.NeedsSave(), w.LastVersion())
	}
}

func TestWhatsNewOnAnOlderInstallShowsThisVersionOnce(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.8.0", "", true)
	view := w.View()
	if view == nil || !view.Show || view.Since != "" {
		t.Fatalf("view = %+v", view)
	}
	if got := versions(w); len(got) != 1 || got[0] != "0.8.0" {
		t.Fatalf("versions = %v", got)
	}
	if !w.NeedsSave() {
		t.Fatal("not saved")
	}
}

func TestWhatsNewDismissHidesIt(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.8.0", "0.7.0", true)
	w.Dismiss()
	if view := w.View(); view == nil || view.Show || len(view.Changes) != 2 {
		t.Fatalf("view = %+v", view)
	}
}

func TestWhatsNewAfterADowngradeShowsNothingAndRemembersTheRunningVersion(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.7.1", "0.8.0", true)
	if view := w.View(); view == nil || view.Show {
		t.Fatalf("view = %+v", view)
	}
	if !w.NeedsSave() || w.LastVersion() != "0.7.1" {
		t.Fatalf("save = %v %q", w.NeedsSave(), w.LastVersion())
	}
}

func TestWhatsNewInADevelopmentBuildShowsNothingAndKeepsTheSavedVersion(t *testing.T) {
	for _, running := range []string{"dev", "v0.8.0-3-gabc1234-dirty", "0.0.0-edge.abc1234"} {
		w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), running, "0.7.0", true)
		if w.View() != nil || w.NeedsSave() || w.LastVersion() != "0.7.0" {
			t.Errorf("%s: view %+v, save %v %q", running, w.View(), w.NeedsSave(), w.LastVersion())
		}
	}
}

func TestWhatsNewWithoutASectionForTheRunningVersionShowsNothing(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.9.0", "0.8.0", true)
	if w.View() != nil {
		t.Fatalf("view = %+v", w.View())
	}
	if !w.NeedsSave() || w.LastVersion() != "0.9.0" {
		t.Fatalf("save = %v %q", w.NeedsSave(), w.LastVersion())
	}
}

func TestWhatsNewTreatsAnUnreadableSavedVersionAsAnOlderInstall(t *testing.T) {
	w := update.NewWhatsNew(update.ParseChangelog(sampleChangelog), "v0.8.0", "next", true)
	if view := w.View(); view == nil || !view.Show || len(view.Changes) != 1 {
		t.Fatalf("view = %+v", view)
	}
}

func TestChangelogListsEveryReleasedVersionNewestFirst(t *testing.T) {
	text := "## [0.7.0] - 2026-10-28\n### Added\n- Old.\n## [Unreleased]\n### Added\n- Soon.\n## 0.8.0 - 2026-11-02\n### Added\n- New.\n## [0.7.1]\n### Fixed\n- Fix.\n"
	got := update.NewWhatsNew(update.ParseChangelog(text), "v0.8.0", "0.8.0", true).Changelog()
	if len(got) != 3 || got[0].Version != "0.8.0" || got[1].Version != "0.7.1" || got[2].Version != "0.7.0" {
		t.Fatalf("changelog = %+v", got)
	}
	if got[0].Date != "2026-11-02" || got[1].Date != "" {
		t.Fatalf("dates = %q %q", got[0].Date, got[1].Date)
	}
}
