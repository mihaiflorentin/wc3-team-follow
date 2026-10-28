//go:build darwin

package updater

// system is the running system, for the player's messages.
const system = "darwin"

// locate finds the app bundle that holds the running program.
func locate(exe string) (target, error) { return locateBundle(exe) }

// replace swaps the whole app bundle, so the icon, Info.plist and signature
// match the new version.
func replace(t target, unpacked, staging string) error { return swapBundle(t.path, unpacked, staging) }
