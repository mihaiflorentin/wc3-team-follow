//go:build !linux && !windows && !darwin

package discovery

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("Game discovery is not supported on this operating system.")

type unsupported struct{}

func newPlatform() platform { return unsupported{} }

func (unsupported) findGame(context.Context) (game, error)         { return game{}, errUnsupported }
func (unsupported) listeners(context.Context, game) ([]int, error) { return nil, errUnsupported }
func (unsupported) session(context.Context, game) (string, error)  { return "", errUnsupported }
func (unsupported) version(context.Context, game) string           { return "" }
