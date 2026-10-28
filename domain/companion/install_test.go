package companion_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	mockupdater "github.com/mihaiflorentin/wc3-team-follow/mock/updater"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// offering runs a companion on v0.7.0 that offers v0.8.0.
func offering(t *testing.T, script mockupdater.Install) *harness {
	t.Helper()
	h := newUpdateHarness(t, updatePrefs(true, ""), "v0.7.0")
	h.updater.Set(newerRelease("v0.8.0"), nil)
	h.updater.Script(script)
	h.run(t)
	waitFor(t, "banner", func() bool { return h.svc.State().Update.Prompt })
	return h
}

func TestUpdateDownloadsInstallsAndRestarts(t *testing.T) {
	hold := make(chan struct{})
	h := offering(t, mockupdater.Install{Progress: []dto.UpdateProgress{{Done: 10, Total: 100}}, Hold: hold, Path: "/tmp/update/file.tar.gz"})
	s, err := h.svc.InstallUpdate()
	if err != nil || s.Update.Phase != dto.UpdateDownloading {
		t.Fatalf("InstallUpdate = %+v, %v", s.Update, err)
	}
	waitFor(t, "progress", func() bool { p := h.svc.State().Update.Progress; return p != nil && p.Done == 10 })
	close(hold)
	waitFor(t, "restart", func() bool { return h.svc.State().Update.Phase == dto.UpdateRestarting })
	if got := h.updater.Downloaded(); len(got) != 1 || got[0] != "v0.8.0" {
		t.Fatalf("downloaded %v", got)
	}
	if got := h.updater.Installed(); len(got) != 1 || got[0] != "/tmp/update/file.tar.gz" {
		t.Fatalf("installed %v", got)
	}
}

func TestAFailedDownloadInstallsNothingAndSaysWhy(t *testing.T) {
	h := offering(t, mockupdater.Install{DownloadErr: errors.New("connection reset")})
	if _, err := h.svc.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failure", func() bool { return h.svc.State().Update.Phase == dto.UpdateFailed })
	if u := h.svc.State().Update; !strings.Contains(u.Problem, "keeps running") || u.Release == nil || !u.Prompt {
		t.Fatalf("update = %+v", u)
	}
	if got := h.updater.Installed(); len(got) != 0 {
		t.Fatalf("installed %v after a failed download", got)
	}
}

func TestAFailedInstallKeepsTheOldVersionRunning(t *testing.T) {
	h := offering(t, mockupdater.Install{Path: "/tmp/update/file.zip", InstallErr: errors.New("rename: access denied")})
	if _, err := h.svc.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failure", func() bool { return h.svc.State().Update.Phase == dto.UpdateFailed })
	if u := h.svc.State().Update; !strings.Contains(u.Problem, "couldn't replace its files") {
		t.Fatalf("update = %+v", u)
	}
}

func TestCancelStopsTheDownload(t *testing.T) {
	hold := make(chan struct{})
	defer close(hold)
	h := offering(t, mockupdater.Install{Hold: hold, Path: "/tmp/x"})
	if _, err := h.svc.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	h.svc.CancelUpdate()
	waitFor(t, "back to the offer", func() bool { return h.svc.State().Update.Phase == dto.UpdateAvailable })
	if u := h.svc.State().Update; u.Problem != "" || !u.Prompt {
		t.Fatalf("update = %+v", u)
	}
	if got := h.updater.Installed(); len(got) != 0 {
		t.Fatalf("installed %v after Cancel", got)
	}
}

func TestDevelopmentBuildsNeverInstall(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(true, ""), "dev")
	h.run(t)
	if _, err := h.svc.InstallUpdate(); err == nil {
		t.Fatal("a development build installed an update")
	}
	time.Sleep(1200 * time.Millisecond)
	if len(h.updater.Downloaded()) != 0 || h.updater.Cleanups() != 0 {
		t.Fatal("a development build touched its files")
	}
}

func TestEveryStartCleansUpAfterAnEarlierUpdate(t *testing.T) {
	h := newUpdateHarness(t, updatePrefs(false, ""), "v0.8.0")
	h.run(t)
	waitFor(t, "cleanup", func() bool { return h.updater.Cleanups() == 1 })
}

func TestTheSetcapStepShowsAfterAnUpdateUntilDone(t *testing.T) {
	h := newHarnessWith(t, updatePrefs(false, ""), mockrecent.New(), func(o *companion.Options) {
		o.AppVersion, o.ReleasesURL = "v0.8.0", releasePage
		o.System, o.Executable, o.Updated = "linux", "/opt/wc3-team-follow", true
	})
	after := h.svc.State().Update.After
	if after == nil || after.Setcap != "sudo setcap cap_sys_ptrace=ep '/opt/wc3-team-follow'" {
		t.Fatalf("after = %+v", after)
	}
	if s := h.svc.DismissUpdateNotes(); s.Update.After != nil {
		t.Fatalf("after Done = %+v", s.Update.After)
	}
}
