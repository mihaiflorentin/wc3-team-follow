// Package companion holds a test double for contract.Companion.
package companion

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.Companion = (*Companion)(nil)

// Companion returns fixed values and records the last call.
type Companion struct {
	Current      dto.FollowState
	Report       dto.Diagnostics
	Err          error
	LastConfig   dto.FollowConfig
	LastPassword string
	LastHost     dto.HostConfig
	Folders      []dto.MapsFolder
	Listing      dto.MapListing
	LastFolder   string
	LastQuery    string
	// CloseCalls counts CloseGames calls.
	CloseCalls int
	// PermissionCalls counts RequestKeyboardPermission calls.
	PermissionCalls int
	// ClearRecentCalls counts ClearRecentPlayers calls.
	ClearRecentCalls int
	// ClearRankingCalls counts ClearRanking calls.
	ClearRankingCalls int
	// MapChecks counts CheckMaps calls.
	MapChecks int
	// CheckCalls, LaterCalls and SkipCalls count the update calls.
	CheckCalls int
	LaterCalls int
	SkipCalls  int
	// InstallCalls, CancelCalls and DismissNotesCalls count the in-app
	// update calls.
	InstallCalls      int
	CancelCalls       int
	DismissNotesCalls int
	// DismissCalls counts DismissWhatsNew calls; ChangelogSections is what
	// Changelog returns.
	DismissCalls      int
	ChangelogSections []dto.ChangelogSection
	// NoteCalls counts SetNote calls; LastNoteTag and LastNoteText hold the
	// last one's arguments.
	NoteCalls    int
	LastNoteTag  string
	LastNoteText string
}

func (c *Companion) CheckMaps() (dto.FollowState, error) {
	c.MapChecks++
	return c.Current, c.Err
}

func (c *Companion) CheckForUpdates() (dto.FollowState, error) {
	c.CheckCalls++
	return c.Current, c.Err
}

func (c *Companion) InstallUpdate() (dto.FollowState, error) {
	c.InstallCalls++
	return c.Current, c.Err
}

func (c *Companion) CancelUpdate() dto.FollowState {
	c.CancelCalls++
	return c.Current
}

func (c *Companion) DismissUpdateNotes() dto.FollowState {
	c.DismissNotesCalls++
	return c.Current
}

func (c *Companion) UpdateLater() dto.FollowState {
	c.LaterCalls++
	return c.Current
}

func (c *Companion) SkipUpdate() (dto.FollowState, error) {
	c.SkipCalls++
	return c.Current, c.Err
}

func (c *Companion) DismissWhatsNew() dto.FollowState {
	c.DismissCalls++
	return c.Current
}

func (c *Companion) Changelog() []dto.ChangelogSection { return c.ChangelogSections }

func (c *Companion) SetNote(tag, text string) (dto.FollowState, error) {
	c.NoteCalls++
	c.LastNoteTag, c.LastNoteText = tag, text
	return c.Current, c.Err
}

func (c *Companion) ClearRecentPlayers() (dto.FollowState, error) {
	c.ClearRecentCalls++
	return c.Current, c.Err
}

func (c *Companion) ClearRanking() (dto.FollowState, error) {
	c.ClearRankingCalls++
	return c.Current, c.Err
}

func (c *Companion) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *Companion) State() dto.FollowState { return c.Current }

func (c *Companion) Configure(cfg dto.FollowConfig) (dto.FollowState, error) {
	c.LastConfig = cfg
	return c.Current, c.Err
}

func (c *Companion) SubmitPassword(password string) (dto.FollowState, error) {
	c.LastPassword = password
	return c.Current, c.Err
}

func (c *Companion) Diagnostics() dto.Diagnostics { return c.Report }

func (c *Companion) Subscribe(func(dto.FollowState)) func() { return func() {} }

func (c *Companion) ConfigureHost(_ context.Context, cfg dto.HostConfig) (dto.FollowState, error) {
	c.LastHost = cfg
	return c.Current, c.Err
}

func (c *Companion) MapsFolders(context.Context) []dto.MapsFolder { return c.Folders }

func (c *Companion) SetMapsFolder(path string) (dto.FollowState, error) {
	c.LastFolder = path
	return c.Current, c.Err
}

func (c *Companion) BrowseMaps(_ context.Context, dir string) (dto.MapListing, error) {
	c.Listing.Dir = dir
	return c.Listing, c.Err
}

func (c *Companion) SearchMaps(_ context.Context, query string) (dto.MapListing, error) {
	c.LastQuery = query
	return c.Listing, c.Err
}

func (c *Companion) CloseGames(context.Context) (dto.FollowState, error) {
	c.CloseCalls++
	return c.Current, c.Err
}

func (c *Companion) RequestKeyboardPermission(context.Context) (dto.FollowState, error) {
	c.PermissionCalls++
	return c.Current, c.Err
}
