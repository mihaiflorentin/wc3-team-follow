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
