//go:build !linux && !windows && !darwin

package discovery

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("Game discovery is not supported on this operating system.")

type unsupported struct{}

func newPlatform() platform { return unsupported{} }

func (unsupported) findGames(context.Context) ([]game, error)      { return nil, errUnsupported }
func (unsupported) listeners(context.Context, game) ([]int, error) { return nil, errUnsupported }
func (unsupported) session(context.Context, game) (string, error)  { return "", errUnsupported }
func (unsupported) version(context.Context, game) string           { return "" }
func (unsupported) replayFolders(context.Context, *game) []string  { return nil }
func (unsupported) closable(game) bool                             { return false }
func (unsupported) stop(game, bool) error                          { return errUnsupported }
