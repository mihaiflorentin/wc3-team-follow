//go:build windows

package updater

// system is the running system, for the player's messages.
const system = "windows"

// locate finds the running program.
func locate(exe string) (target, error) { return locateProgram(exe) }

// replace moves the running .exe aside and the new one in.
func replace(t target, unpacked, _ string) error { return replaceExe(t.path, unpacked) }
