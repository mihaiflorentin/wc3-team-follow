package update_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

type problem struct{ text string }

func (p problem) Error() string   { return "updater: " + p.text }
func (p problem) Problem() string { return p.text }

func newCheckerOn(c *clock, system, version string, updated bool) *update.Checker {
	return update.New(update.Options{Version: version, Interval: 6 * time.Hour, RetryInterval: 30 * time.Minute, ReleasesURL: releases, Now: c.Now,
		System: system, Executable: "/home/player/Apps/Team Follow/wc3-team-follow", Updated: updated}, true, "")
}

// offered returns a checker on v0.7.0 that offers v0.8.0.
func offered(t *testing.T) *update.Checker {
	t.Helper()
	u := newChecker(newClock(), "v0.7.0", true, "")
	check(t, u, false, release("v0.8.0"), nil)
	return u
}

func TestInstallGoesThroughDownloadingVerifyingInstallingAndRestarting(t *testing.T) {
	u := offered(t)
	rel, err := u.StartInstall()
	if err != nil || rel.Tag != "v0.8.0" || rel.AssetURL == "" || rel.ChecksumURL == "" {
		t.Fatalf("StartInstall = %+v, %v", rel, err)
	}
	if s := u.State(idle()); s.Phase != dto.UpdateDownloading || !s.Prompt || s.Progress == nil || s.Progress.Done != 0 {
		t.Fatalf("after start: %+v", s)
	}
	u.Progress(dto.UpdateProgress{Done: 512, Total: 2048})
	if s := u.State(idle()); s.Phase != dto.UpdateDownloading || s.Progress.Done != 512 || s.Progress.Total != 2048 {
		t.Fatalf("downloading: %+v", s)
	}
	u.Progress(dto.UpdateProgress{Done: 2048, Total: 2048, Verifying: true})
	if s := u.State(idle()); s.Phase != dto.UpdateVerifying {
		t.Fatalf("verifying: %+v", s)
	}
	u.Installing()
	if s := u.State(idle()); s.Phase != dto.UpdateInstalling || s.Progress != nil {
		t.Fatalf("installing: %+v", s)
	}
	u.Restarting()
	if s := u.State(idle()); s.Phase != dto.UpdateRestarting || s.Release.Version != "0.8.0" {
		t.Fatalf("restarting: %+v", s)
	}
}

func TestOnlyOneInstallRunsAndChecksWaitForIt(t *testing.T) {
	u := offered(t)
	if _, err := u.StartInstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := u.StartInstall(); err == nil {
		t.Fatal("a second install started")
	}
	if u.Begin(true) {
		t.Fatal("a check started during the install")
	}
	u.Installing()
	if u.Due() || u.Begin(false) {
		t.Fatal("a check started while installing")
	}
}

func TestNothingToInstallWithoutAnOfferOrInADevelopmentBuild(t *testing.T) {
	u := newChecker(newClock(), "v0.7.0", true, "")
	if _, err := u.StartInstall(); err == nil || !strings.Contains(err.Error(), "no update") {
		t.Fatalf("installed without an offer: %v", err)
	}
	dev := newChecker(newClock(), "v0.7.0-3-gabc1234-dirty", true, "")
	if _, err := dev.StartInstall(); err == nil || !strings.Contains(err.Error(), "development builds") {
		t.Fatalf("a development build installed: %v", err)
	}
}

func TestAFailureSaysWhyAndKeepsTheOfferForAnotherTry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps func(u *update.Checker)
		err   error
		want  string
	}{
		{"download", func(*update.Checker) {}, errors.New("read: connection reset"), "The download didn't finish"},
		{"verify", func(u *update.Checker) { u.Progress(dto.UpdateProgress{Done: 9, Total: 9, Verifying: true}) },
			problem{"The download didn't match its published checksum, so nothing was installed."}, "didn't match its published checksum"},
		{"install", func(u *update.Checker) { u.Installing() }, errors.New("rename: permission denied"), "couldn't replace its files"},
		{"translocated", func(*update.Checker) {}, problem{"Move Warcraft Team Follow to the Applications folder."}, "Move Warcraft Team Follow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := offered(t)
			if _, err := u.StartInstall(); err != nil {
				t.Fatal(err)
			}
			tc.steps(u)
			u.Fail(tc.err)
			s := u.State(idle())
			if s.Phase != dto.UpdateFailed || !s.Prompt || !strings.Contains(s.Problem, tc.want) || !strings.Contains(s.Problem, "0.7.0 keeps running") ||
				strings.Contains(s.Problem, "updater:") || s.Release == nil || s.Progress != nil {
				t.Fatalf("failed state = %+v", s)
			}
			// Try again starts over.
			if _, err := u.StartInstall(); err != nil || u.State(idle()).Problem != "" {
				t.Fatalf("retry: %v, %+v", err, u.State(idle()))
			}
		})
	}
}

func TestCancelGoesBackToTheOffer(t *testing.T) {
	u := offered(t)
	if _, err := u.StartInstall(); err != nil {
		t.Fatal(err)
	}
	u.Progress(dto.UpdateProgress{Done: 1, Total: 10})
	u.Cancelled()
	if s := u.State(idle()); s.Phase != dto.UpdateAvailable || !s.Prompt || s.Problem != "" || s.Progress != nil {
		t.Fatalf("after cancel: %+v", s)
	}
}

func TestLaterDismissesAFailure(t *testing.T) {
	u := offered(t)
	_, _ = u.StartInstall()
	u.Fail(errors.New("boom"))
	u.Later()
	if s := u.State(idle()); s.Phase != dto.UpdateAvailable || s.Prompt || s.Problem != "" {
		t.Fatalf("after Later: %+v", s)
	}
}

func TestInstallNoteSaysWhatToRedoOnThisSystem(t *testing.T) {
	for system, want := range map[string]string{"darwin": "Accessibility", "linux": "setcap", "windows": ""} {
		u := newCheckerOn(newClock(), system, "v0.7.0", false)
		check(t, u, false, release("v0.8.0"), nil)
		note := u.State(idle()).InstallNote
		if (want == "" && note != "") || !strings.Contains(note, want) {
			t.Errorf("%s: note %q", system, note)
		}
	}
}

func TestAfterAnUpdateTheSetupStepIsShownUntilDismissed(t *testing.T) {
	linux := newCheckerOn(newClock(), "linux", "v0.8.0", true)
	after := linux.State(idle()).After
	if after == nil || after.Version != "0.8.0" || after.Setcap != "sudo setcap cap_sys_ptrace=ep '/home/player/Apps/Team Follow/wc3-team-follow'" || after.Accessibility {
		t.Fatalf("linux after = %+v", after)
	}
	linux.DismissAfter()
	if linux.State(idle()).After != nil {
		t.Fatal("still shown after Done")
	}
	if a := newCheckerOn(newClock(), "darwin", "v0.8.0", true).State(idle()).After; a == nil || !a.Accessibility || a.Setcap != "" {
		t.Fatalf("macOS after = %+v", a)
	}
	if a := newCheckerOn(newClock(), "windows", "v0.8.0", true).State(idle()).After; a != nil {
		t.Fatalf("windows has nothing to redo: %+v", a)
	}
	if a := newCheckerOn(newClock(), "linux", "v0.8.0", false).State(idle()).After; a != nil {
		t.Fatalf("shown without an update: %+v", a)
	}
}
