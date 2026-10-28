package gui

import (
	"errors"
	"net/url"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// CheckForUpdates checks GitHub for a newer release now (ADR 0009).
func (b *Bridge) CheckForUpdates() (dto.FollowState, error) { return b.companion.CheckForUpdates() }

// UpdateLater hides the update banner for now.
func (b *Bridge) UpdateLater() dto.FollowState { return b.companion.UpdateLater() }

// SkipUpdate stops offering the release on offer.
func (b *Bridge) SkipUpdate() (dto.FollowState, error) { return b.companion.SkipUpdate() }

// SetCheckUpdates switches automatic update checks on or off.
func (b *Bridge) SetCheckUpdates(on bool) (dto.FollowState, error) {
	return b.companion.Configure(dto.FollowConfig{CheckUpdates: &on})
}

// Update downloads, verifies and installs the release on offer, then
// restarts Team Follow into it; the steps arrive with later states.
func (b *Bridge) Update() (dto.FollowState, error) { return b.companion.InstallUpdate() }

// CancelUpdate stops the update's download.
func (b *Bridge) CancelUpdate() dto.FollowState { return b.companion.CancelUpdate() }

// DismissUpdateNotes hides what the player had to redo after an update.
func (b *Bridge) DismissUpdateNotes() dto.FollowState { return b.companion.DismissUpdateNotes() }

// OpenReleasesPage opens Team Follow's releases page in the browser, for
// updating by hand.
func (b *Bridge) OpenReleasesPage() error {
	return b.open(b.companion.State().Update.ReleasesURL)
}

// open shows a page on github.com in the browser. The page never passes the
// address: it comes from the companion's state, and only GitHub's pages over
// HTTPS are opened.
func (b *Bridge) open(link string) error {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || strings.ContainsAny(link, " \t\r\n") {
		return errors.New("That page isn't on GitHub, so it wasn't opened.")
	}
	if err := b.openURL(link); err != nil {
		return errors.New("The browser couldn't be opened. Visit " + link)
	}
	return nil
}

// browserOpener opens links with the window's runtime.
func (b *Bridge) browserOpener(link string) error {
	if b.ctx == nil {
		return errors.New("the browser is only available in the app")
	}
	wailsruntime.BrowserOpenURL(b.ctx, link)
	return nil
}
