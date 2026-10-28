package update

import (
	"errors"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Sentences the window shows in the Updates section.
const (
	msgDevBuild    = "Updates are off in development builds. Download releases from GitHub."
	msgOffline     = "Team Follow couldn't check for updates. Check your internet connection and try again."
	msgRateLimited = "GitHub is limiting update checks right now. Try again in a few minutes."
	msgUpToDate    = "Team Follow is up to date."
)

// Options tunes the checker.
type Options struct {
	// Version is the running build's version. Only a clean release
	// version (vX.Y.Z) ever checks.
	Version string
	// Interval is the time between automatic checks; RetryInterval the
	// time before another try after a failed one.
	Interval      time.Duration
	RetryInterval time.Duration
	// ReleasesURL is the releases page, for updating by hand.
	ReleasesURL string
	Now         func() time.Time
	// System is the operating system (runtime.GOOS), for what the player
	// redoes after an update; Executable is this program's path, for the
	// Linux setcap command.
	System     string
	Executable string
	// Updated marks the first start after an update.
	Updated bool
}

// Activity is what the player is doing, for the restart warning.
type Activity struct {
	InLobby, InMatch   bool
	AutoJoin, AutoHost bool
}

// Checker decides when to check for a newer release and what to offer.
// It is not safe for concurrent use; the companion calls it under its lock.
//
// The phases are idle, checking and available, then, once the player
// clicks Update, downloading, verifying, installing and restarting, or
// failed (install.go).
type Checker struct {
	opts      Options
	current   Version
	supported bool

	automatic bool
	skipped   string

	checking bool
	// manual is set while a check the player asked for runs.
	manual bool
	next   time.Time
	// release is the newer release on offer, or nil.
	release     *dto.UpdateRelease
	later       bool
	message     string
	lastChecked time.Time
	// offer is the release on offer as the Updater found it, for Download.
	offer dto.Release

	// install is the in-app update's phase, "" before the player clicks
	// Update; progress the download's, problem why it failed.
	install  string
	progress dto.UpdateProgress
	problem  string
	// after is what the player may redo after an update, until dismissed.
	after *dto.UpdateAfter
}

// New returns a checker for the running version, with the saved
// "Check automatically" setting and skipped version. The first automatic
// check is due at once.
func New(opts Options, automatic bool, skipped string) *Checker {
	current, ok := ParseVersion(opts.Version)
	c := &Checker{opts: opts, current: current, supported: ok, automatic: automatic, skipped: skipped}
	if !ok {
		c.message = msgDevBuild
	}
	if ok && opts.Updated {
		c.after = afterUpdate(opts, current)
	}
	return c
}

// Supported reports whether this build checks for updates at all.
func (c *Checker) Supported() bool { return c.supported }

// Due reports whether an automatic check should start now.
func (c *Checker) Due() bool {
	return c.supported && c.automatic && !c.checking && !c.installing() && !c.opts.Now().Before(c.next)
}

// Begin starts a check, automatic or asked for by the player. It reports
// false when this build never checks, an update is being installed or a
// check already runs; a player's click during a running check still gets
// that check's answer.
func (c *Checker) Begin(manual bool) bool {
	if !c.supported || c.installing() {
		return false
	}
	if c.checking {
		c.manual = c.manual || manual
		return false
	}
	c.checking, c.manual = true, manual
	if manual {
		c.message, c.later = "", false
	}
	return true
}

// Finish records the Updater's answer to the running check. A failure is
// quiet unless the player asked, and keeps whatever was found before; the
// next automatic check waits RetryInterval, or until GitHub's rate limit
// lifts.
func (c *Checker) Finish(rel dto.Release, err error) {
	manual := c.manual
	c.checking, c.manual = false, false
	now := c.opts.Now()
	if err != nil {
		c.next = now.Add(c.opts.RetryInterval)
		var retry contract.RetryLater
		if errors.As(err, &retry) {
			if at := retry.RetryAt(); at.After(c.next) {
				c.next = at
			}
			if manual {
				c.message = msgRateLimited
			}
		} else if manual {
			c.message = msgOffline
		}
		return
	}
	c.next, c.lastChecked = now.Add(c.opts.Interval), now
	offered, ok := ParseVersion(rel.Tag)
	if !ok || !offered.Newer(c.current) {
		c.release = nil
		if manual {
			c.message = msgUpToDate
		}
		return
	}
	if rel.AssetURL == "" || rel.ChecksumURL == "" {
		c.release = nil
		if manual {
			c.message = "Team Follow " + offered.String() + " is out, but has no download for this system yet."
		}
		return
	}
	if c.release == nil || c.release.Version != offered.String() {
		c.later = false
		c.endInstall()
	}
	c.offer = rel
	c.message = ""
	c.release = &dto.UpdateRelease{
		Version:   offered.String(),
		URL:       rel.PageURL,
		Published: rel.Published,
		Changes:   ChangesBetween(ParseChangelog(rel.Changelog), c.current, offered),
		Notes:     PlainNotes(rel.Notes),
	}
}

// Later hides the banner for the release on offer until a newer one is
// found or the player checks again, and dismisses a failed update. Nothing
// is saved. It does nothing while an update is being installed.
func (c *Checker) Later() {
	if c.installing() {
		return
	}
	c.later = true
	c.endInstall()
}

// Skip silences the release on offer for good, and returns its version
// for saving. A newer release is offered again.
func (c *Checker) Skip() (string, error) {
	if c.release == nil {
		return "", errors.New("There is no update to skip.")
	}
	if c.installing() {
		return "", errors.New("The update is already being installed.")
	}
	c.endInstall()
	c.skipped = c.release.Version
	return c.skipped, nil
}

// SetAutomatic switches automatic checks on or off.
func (c *Checker) SetAutomatic(on bool) { c.automatic = on }

// Automatic reports whether automatic checks are on.
func (c *Checker) Automatic() bool { return c.automatic }

// SkippedVersion is the version the player skipped, or "".
func (c *Checker) SkippedVersion() string { return c.skipped }

// Release returns the release on offer.
func (c *Checker) Release() (dto.UpdateRelease, bool) {
	if c.release == nil {
		return dto.UpdateRelease{}, false
	}
	return *c.release, true
}

// State is the update part of FollowState, with the restart warning for
// what the player is doing.
func (c *Checker) State(activity Activity) dto.UpdateState {
	s := dto.UpdateState{
		Supported:          c.supported,
		Phase:              dto.UpdateIdle,
		CurrentVersion:     c.opts.Version,
		CheckAutomatically: c.automatic,
		Message:            c.message,
		ReleasesURL:        c.opts.ReleasesURL,
	}
	if c.supported {
		s.CurrentVersion = c.current.String()
	}
	if !c.lastChecked.IsZero() {
		at := c.lastChecked
		s.LastChecked = &at
	}
	if c.release != nil {
		rel := *c.release
		s.Release = &rel
		s.Phase = dto.UpdateAvailable
		s.Skipped = rel.Version == c.skipped
		s.Prompt = !s.Skipped && !c.later
		s.Warning = warning(activity)
	}
	if c.checking {
		s.Phase = dto.UpdateChecking
	}
	c.installState(&s)
	return s
}

// warning says what updating interrupts: restarting turns Follow off, as
// every start does.
func warning(a Activity) string {
	const restart = " Updating restarts Team Follow and turns Follow off."
	switch {
	case a.InLobby || a.InMatch:
		return "You're in a lobby or a match." + restart
	case a.AutoJoin && a.AutoHost:
		return "Auto-join and Auto-host are on." + restart
	case a.AutoJoin:
		return "Auto-join is on." + restart
	case a.AutoHost:
		return "Auto-host is on." + restart
	}
	return ""
}
