//go:build linux

package discovery

import "os"

func newPlatform() platform {
	home, _ := os.UserHomeDir()
	return procfs{root: "/proc", home: home}
}
