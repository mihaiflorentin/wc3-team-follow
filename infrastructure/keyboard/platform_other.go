//go:build !windows && !darwin

package keyboard

import (
	"errors"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Linux runs the game under Wine or Proton, where keys must come from a
// helper inside the game's Wine prefix (ADR 0007). Until that helper is
// live-tested, typing is not supported here and nothing is ever pressed.
const unsupportedReason = "Typing in the match chat isn't supported on Linux yet."

type unsupported struct{}

func newSystem() system { return unsupported{} }

func (unsupported) status() dto.KeyboardStatus { return dto.KeyboardStatus{Reason: unsupportedReason} }
func (unsupported) requestPermission() error   { return errors.New(unsupportedReason) }
func (unsupported) check(string) error         { return errors.New(unsupportedReason) }
func (unsupported) frontmostPID() (int, error) { return 0, errors.New(unsupportedReason) }
func (unsupported) press(key) error            { return errors.New(unsupportedReason) }
