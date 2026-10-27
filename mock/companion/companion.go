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
