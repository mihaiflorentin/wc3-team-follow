package update

import (
	"slices"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// WhatsNew decides what "What's new" shows on start-up, from the changelog
// built into the app and the version the player ran last (saved in the
// preferences). It is not safe for concurrent use; the companion calls it
// under its lock.
type WhatsNew struct {
	changelog []dto.ChangelogSection
	view      *dto.WhatsNew
	last      string
	save      bool
}

// NewWhatsNew decides for the running version and the last version run
// ("" when none was saved); stored reports that settings were saved
// before, so an empty last version means an install older than this
// feature rather than a fresh one.
//
//   - A newer version than the last one run shows every section after
//     the last one, up to and including this one.
//   - An older install shows this version's entries once.
//   - A fresh install, a second start and a downgrade show nothing.
//   - A build that isn't a clean release version shows nothing and keeps
//     the saved version.
//
// What's new can always be reopened with this version's entries, or the
// ones it showed.
func NewWhatsNew(changelog []dto.ChangelogSection, running, last string, stored bool) *WhatsNew {
	w := &WhatsNew{changelog: newestFirst(slices.Clone(changelog)), last: last}
	current, ok := ParseVersion(running)
	if !ok {
		return w
	}
	w.last, w.save = current.String(), last != current.String()
	previous, known := ParseVersion(last)
	show := w.save && (stored || last != "") && (!known || current.Newer(previous))

	since := ""
	var changes []dto.ChangelogSection
	if show && known {
		since, changes = previous.String(), ChangesBetween(w.changelog, previous, current)
	} else {
		changes = slices.DeleteFunc(slices.Clone(w.changelog), func(s dto.ChangelogSection) bool { return s.Version != current.String() })
	}
	if len(changes) > 0 {
		w.view = &dto.WhatsNew{Version: current.String(), Since: since, Changes: changes, Show: show}
	}
	return w
}

// View is What's new for the window, or nil when the changelog has nothing
// for this version.
func (w *WhatsNew) View() *dto.WhatsNew {
	if w.view == nil {
		return nil
	}
	v := *w.view
	return &v
}

// Dismiss closes What's new; it stays closed until the next update.
func (w *WhatsNew) Dismiss() {
	if w.view != nil {
		w.view.Show = false
	}
}

// NeedsSave reports that the last version run changed and should be saved
// at start-up.
func (w *WhatsNew) NeedsSave() bool { return w.save }

// LastVersion is the version to save as the last one run.
func (w *WhatsNew) LastVersion() string { return w.last }

// Changelog is every released version in the built-in changelog, newest
// first.
func (w *WhatsNew) Changelog() []dto.ChangelogSection { return slices.Clone(w.changelog) }
