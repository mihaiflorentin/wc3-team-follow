package update

import (
	"errors"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// The in-app update (ADR 0009): after the player clicks Update, the
// companion downloads, verifies and installs the release on offer and
// reports each step here. A failure leaves the running version untouched
// and keeps the offer, so the player can try again.

// StartInstall begins installing the release on offer and returns it for
// Download. It refuses in a development build, without an offer, and while
// an update is already being installed.
func (c *Checker) StartInstall() (dto.Release, error) {
	switch {
	case !c.supported:
		return dto.Release{}, errors.New("Updates are off in development builds.")
	case c.release == nil:
		return dto.Release{}, errors.New("There is no update to install.")
	case c.installing():
		return dto.Release{}, errors.New("The update is already being installed.")
	}
	c.install, c.progress, c.problem, c.later = dto.UpdateDownloading, dto.UpdateProgress{}, "", false
	return c.offer, nil
}

// Progress records the download's progress; once the file is complete,
// Verifying moves on to checking its SHA-256.
func (c *Checker) Progress(p dto.UpdateProgress) {
	if c.install != dto.UpdateDownloading && c.install != dto.UpdateVerifying {
		return
	}
	c.progress = p
	if p.Verifying {
		c.install = dto.UpdateVerifying
	}
}

// Installing records that the verified file is replacing the app.
func (c *Checker) Installing() { c.install = dto.UpdateInstalling }

// Restarting records that the new version is installed and Team Follow
// restarts into it.
func (c *Checker) Restarting() { c.install = dto.UpdateRestarting }

// Cancelled goes back to the offer after the player cancelled the download.
func (c *Checker) Cancelled() { c.install, c.progress, c.problem = "", dto.UpdateProgress{}, "" }

// Fail records why the update failed, as sentences for the window. An
// Updater error the player can act on (contract.UpdateProblem) says so in
// its own words; anything else gets the step that failed.
func (c *Checker) Fail(err error) {
	reason := ""
	var p contract.UpdateProblem
	switch {
	case errors.As(err, &p):
		reason = p.Problem()
	case c.install == dto.UpdateInstalling || c.install == dto.UpdateRestarting:
		reason = "Team Follow couldn't replace its files, so nothing was changed."
	case c.install == dto.UpdateVerifying:
		reason = "The download couldn't be checked, so nothing was installed."
	default:
		reason = "The download didn't finish. Check your internet connection and try again."
	}
	c.install, c.progress = dto.UpdateFailed, dto.UpdateProgress{}
	c.problem = reason + " Team Follow " + c.current.String() + " keeps running. You can also download the new version from the releases page."
}

// Busy reports whether the download can still be cancelled.
func (c *Checker) Busy() bool {
	return c.install == dto.UpdateDownloading || c.install == dto.UpdateVerifying
}

// DismissAfter hides what the player had to redo after an update.
func (c *Checker) DismissAfter() { c.after = nil }

// installing reports whether an update is between the click and the
// restart; checks and a second install wait for it.
func (c *Checker) installing() bool {
	switch c.install {
	case dto.UpdateDownloading, dto.UpdateVerifying, dto.UpdateInstalling, dto.UpdateRestarting:
		return true
	}
	return false
}

// endInstall forgets a failed or cancelled update.
func (c *Checker) endInstall() {
	if !c.installing() {
		c.install, c.progress, c.problem = "", dto.UpdateProgress{}, ""
	}
}

// installState adds the in-app update to the state.
func (c *Checker) installState(s *dto.UpdateState) {
	if c.after != nil {
		after := *c.after
		s.After = &after
	}
	if c.release == nil {
		return
	}
	s.InstallNote = installNote(c.opts.System)
	if c.install == "" {
		return
	}
	s.Phase, s.Problem = c.install, c.problem
	// The banner stays up from the click to the restart; Later hides a
	// failure.
	s.Prompt = true
	if c.install == dto.UpdateDownloading || c.install == dto.UpdateVerifying {
		p := c.progress
		s.Progress = &p
	}
}

// installNote says what the player may need to redo after updating.
func installNote(system string) string {
	switch system {
	case "darwin":
		return "After updating, macOS may ask you again to allow Team Follow under Accessibility for Game mode."
	case "linux":
		return "After updating, run the setcap step again so Team Follow can read the game."
	}
	return ""
}

// afterUpdate is what the player redoes on the first start after an update,
// or nil when there is nothing.
func afterUpdate(opts Options, current Version) *dto.UpdateAfter {
	switch opts.System {
	case "linux":
		return &dto.UpdateAfter{Version: current.String(), Setcap: "sudo setcap cap_sys_ptrace=ep " + shellQuote(opts.Executable)}
	case "darwin":
		return &dto.UpdateAfter{Version: current.String(), Accessibility: true}
	}
	return nil
}

// shellQuote quotes path for a POSIX shell.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
