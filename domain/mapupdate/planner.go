// Package mapupdate decides when to check tracked maps for a newer release,
// what to download and what the Maps section shows (docs/adr/0012). It is
// pure: the companion drives it with its clock and does the I/O.
package mapupdate

import (
	"errors"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Options tunes the planner.
type Options struct {
	// Interval is the time between automatic checks; RetryInterval the
	// time before another try after a failed check or download. Zero uses
	// 6 hours and 30 minutes.
	Interval      time.Duration
	RetryInterval time.Duration
	Now           func() time.Time
}

// row is one tracked map's state.
type row struct {
	tracked   dto.TrackedMap
	phase     string
	installed string
	release   dto.MapRelease
	progress  *dto.MapProgress
	message   string
	// before is the phase to go back to when a quiet check fails.
	before string
}

// Planner decides when to check and what to fetch. It is not safe for
// concurrent use; the companion calls it under its lock.
type Planner struct {
	opts    Options
	rows    []row
	enabled bool

	checking bool
	manual   bool
	// failed marks a check or download that failed, so the next check
	// comes after RetryInterval; retryAt is GitHub's rate limit, if later.
	failed      bool
	retryAt     time.Time
	next        time.Time
	lastChecked time.Time
	message     string
	notes       []string
}

// New returns a planner for the tracked maps, with the saved "Keep maps up
// to date" setting. The first automatic check is due at once.
func New(opts Options, tracked []dto.TrackedMap, enabled bool) *Planner {
	if opts.Interval <= 0 {
		opts.Interval = 6 * time.Hour
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = 30 * time.Minute
	}
	p := &Planner{opts: opts, enabled: enabled}
	for _, t := range tracked {
		p.rows = append(p.rows, row{tracked: t, phase: dto.MapIdle})
	}
	return p
}

// Tracked lists the tracked maps, in the order Found and the other calls
// number them.
func (p *Planner) Tracked() []dto.TrackedMap {
	out := make([]dto.TrackedMap, len(p.rows))
	for i, r := range p.rows {
		out[i] = r.tracked
	}
	return out
}

// Enabled reports whether "Keep maps up to date" is on.
func (p *Planner) Enabled() bool { return p.enabled }

// SetEnabled switches "Keep maps up to date" on, which makes a check due
// at once, or off. A running download is stopped by the companion, which
// then calls Cancelled.
func (p *Planner) SetEnabled(on bool) {
	if on && !p.enabled {
		p.next = time.Time{}
	}
	p.enabled = on
}

// Due reports whether an automatic check should start now.
func (p *Planner) Due() bool {
	return p.enabled && len(p.rows) > 0 && !p.checking && !p.opts.Now().Before(p.next)
}

// Begin starts a check of every tracked map, automatic or asked for by the
// player ("Check now", which also works with the switch off but never
// downloads then). It reports false when there is nothing to check or a
// check already runs; a click during a running check gets its answer.
func (p *Planner) Begin(manual bool) bool {
	if len(p.rows) == 0 {
		return false
	}
	if p.checking {
		// The player's click still gets the running check's answer.
		p.manual = p.manual || manual
		return false
	}
	p.checking, p.manual, p.failed, p.retryAt, p.message = true, manual, false, time.Time{}, ""
	for i := range p.rows {
		r := &p.rows[i]
		r.before = r.phase
		r.phase = dto.MapChecking
		if manual {
			r.message = ""
		}
	}
	return true
}

// Found records map i's newest release and the files found in the Maps
// folder, or why the release couldn't be read. It reports whether to
// download the release: the switch is on and no installed file has its
// version (compared as strings, ignoring case).
func (p *Planner) Found(i int, rel dto.MapRelease, installed []dto.InstalledMap, err error) (download bool) {
	r := &p.rows[i]
	if err != nil {
		p.fail(err)
		var refused contract.UpdateProblem
		switch {
		case errors.As(err, &refused):
			r.phase, r.message = dto.MapFailed, refused.Problem()
		case p.manual:
			r.phase, r.message = dto.MapFailed, checkFailed(r.tracked, err)
		default:
			// Quiet: what was found before stays.
			r.phase = r.before
			if r.phase == dto.MapChecking {
				r.phase = dto.MapIdle
			}
		}
		return false
	}
	r.release = rel
	r.installed = installedVersion(installed, rel.Version)
	r.progress = nil
	switch {
	case rel.Version == "":
		r.phase = dto.MapIdle
		if p.manual {
			r.message = "No release of " + r.tracked.Name + " with a map file was found on GitHub."
		}
		return false
	case has(installed, rel.Version):
		r.phase, r.message = dto.MapUpToDate, ""
		return false
	case !p.enabled:
		r.phase, r.message = dto.MapMissing, title(rel)+" is out. Turn on Keep maps up to date to download it."
		return false
	}
	r.phase, r.message = dto.MapDownloading, ""
	r.progress = &dto.MapProgress{Total: max(rel.Size, 0)}
	p.notes = append(p.notes, "Downloading "+title(rel)+" into your Maps folder.")
	return true
}

// Progress records how far map i's download is.
func (p *Planner) Progress(i int, pr dto.MapProgress) {
	r := &p.rows[i]
	if r.phase == dto.MapDownloading {
		r.progress = &pr
	}
}

// Downloaded records how map i's download ended.
func (p *Planner) Downloaded(i int, _ string, err error) {
	r := &p.rows[i]
	r.progress = nil
	if err != nil {
		p.fail(err)
		reason := "Check your internet connection and try again."
		var refused contract.UpdateProblem
		if errors.As(err, &refused) {
			reason = refused.Problem()
		}
		r.phase, r.message = dto.MapFailed, title(r.release)+" couldn't be downloaded. "+reason
		p.notes = append(p.notes, r.message)
		return
	}
	r.phase, r.installed = dto.MapDone, r.release.Version
	r.message = title(r.release) + " is ready in your Maps folder."
	p.notes = append(p.notes, r.message)
}

// Cancelled records that map i's download stopped because the switch was
// turned off or Team Follow is closing; nothing was kept.
func (p *Planner) Cancelled(i int) {
	r := &p.rows[i]
	r.progress = nil
	r.phase, r.message = dto.MapMissing, title(r.release)+" is out. Turn on Keep maps up to date to download it."
}

// Abort ends the running check before any map was checked, with a sentence
// for the player, such as no Maps folder. It is shown only when the player
// asked; it is retried after RetryInterval.
func (p *Planner) Abort(sentence string) {
	p.failed = true
	for i := range p.rows {
		r := &p.rows[i]
		if r.phase == dto.MapChecking {
			r.phase = r.before
			if r.phase == dto.MapChecking {
				r.phase = dto.MapIdle
			}
		}
	}
	if p.manual {
		p.message = sentence
	}
	p.finish()
}

// Finish ends the running check and schedules the next one.
func (p *Planner) Finish() {
	p.lastChecked = p.opts.Now()
	p.finish()
}

func (p *Planner) finish() {
	now := p.opts.Now()
	p.checking, p.manual = false, false
	if p.failed {
		p.next = now.Add(p.opts.RetryInterval)
		if p.retryAt.After(p.next) {
			p.next = p.retryAt
		}
		return
	}
	p.next = now.Add(p.opts.Interval)
}

// fail marks the check as failed, remembering GitHub's rate limit.
func (p *Planner) fail(err error) {
	p.failed = true
	var retry contract.RetryLater
	if errors.As(err, &retry) && retry.RetryAt().After(p.retryAt) {
		p.retryAt = retry.RetryAt()
	}
}

// TakeNotes returns the Activity lines since the last call: a download
// starting, ready or failed.
func (p *Planner) TakeNotes() []string {
	notes := p.notes
	p.notes = nil
	return notes
}

// Checking reports whether a check (and any download it started) runs.
func (p *Planner) Checking() bool { return p.checking }

// Downloading reports whether a map is being downloaded.
func (p *Planner) Downloading() bool {
	for _, r := range p.rows {
		if r.phase == dto.MapDownloading {
			return true
		}
	}
	return false
}

// State is the Maps section's part of FollowState.
func (p *Planner) State() dto.MapUpdatesState {
	s := dto.MapUpdatesState{Enabled: p.enabled, Checking: p.checking, Message: p.message, Maps: []dto.MapUpdate{}}
	if !p.lastChecked.IsZero() {
		at := p.lastChecked
		s.LastChecked = &at
	}
	for _, r := range p.rows {
		m := dto.MapUpdate{Name: r.tracked.Name, Repository: r.tracked.Repository, Installed: r.installed, Latest: r.release.Version,
			Phase: r.phase, Message: r.message, PageURL: r.release.PageURL}
		if r.progress != nil {
			pr := *r.progress
			m.Progress = &pr
		}
		s.Maps = append(s.Maps, m)
	}
	return s
}

// has reports whether a file with version is installed.
func has(installed []dto.InstalledMap, version string) bool {
	for _, m := range installed {
		if strings.EqualFold(m.Version, version) {
			return true
		}
	}
	return false
}

// installedVersion is the newest version when installed, otherwise the
// highest installed version (Newest), or "".
func installedVersion(installed []dto.InstalledMap, newest string) string {
	for _, m := range installed {
		if newest != "" && strings.EqualFold(m.Version, newest) {
			return m.Version
		}
	}
	if m, ok := Newest(installed); ok {
		return m.Version
	}
	return ""
}

func title(rel dto.MapRelease) string { return rel.Map.Name + " " + rel.Version }

// checkFailed is the sentence for a check the player asked for that failed.
func checkFailed(tracked dto.TrackedMap, err error) string {
	var retry contract.RetryLater
	if errors.As(err, &retry) {
		return "GitHub is limiting checks right now. Try again in a few minutes."
	}
	return "Team Follow couldn't reach GitHub to check " + tracked.Name + ". Check your internet connection and try again."
}
