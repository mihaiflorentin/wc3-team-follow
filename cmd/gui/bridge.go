package gui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

type dtoState = dto.FollowState

// Bridge is the API the page calls as window.go.gui.Bridge. Errors reach
// the page as rejected promises carrying the message, which is written for
// players.
type Bridge struct {
	ctx        context.Context
	companion  contract.Companion
	appVersion string
	executable string
}

// NewBridge returns the bridge for companion.
func NewBridge(companion contract.Companion, appVersion, executable string) *Bridge {
	return &Bridge{companion: companion, appVersion: appVersion, executable: executable}
}

// State returns the current state.
func (b *Bridge) State() dto.FollowState { return b.companion.State() }

// Configure changes the selection or mode, or starts or stops Follow.
func (b *Bridge) Configure(cfg dto.FollowConfig) (dto.FollowState, error) {
	return b.companion.Configure(cfg)
}

// SubmitPassword answers the lobby password request once.
func (b *Bridge) SubmitPassword(password string) (dto.FollowState, error) {
	return b.companion.SubmitPassword(password)
}

// Diagnostics returns the shareable report as indented JSON.
func (b *Bridge) Diagnostics() (string, error) {
	report := b.companion.Diagnostics()
	report.AppVersion = b.appVersion
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode report: %w", err)
	}
	return string(out), nil
}

// ExecutablePath returns this program's path for setup instructions.
func (b *Bridge) ExecutablePath() string { return b.executable }
