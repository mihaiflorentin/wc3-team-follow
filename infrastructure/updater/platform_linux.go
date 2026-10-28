//go:build linux

package updater

// system is the running system, for the player's messages.
const system = "linux"

// locate finds the running program.
func locate(exe string) (target, error) { return locateProgram(exe) }

// replace renames the new program over the old one.
func replace(t target, unpacked, _ string) error { return replaceBinary(t.path, unpacked) }
