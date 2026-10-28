//go:build !darwin && !windows && !linux

package updater

import "errors"

// system is the running system, for the player's messages.
const system = "other"

func locate(string) (target, error) {
	return target{}, problem{sentence: "Team Follow can't update itself on this system."}
}

func replace(target, string, string) error { return errors.New("not supported on this system") }
