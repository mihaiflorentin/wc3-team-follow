//go:build darwin

package discovery

import (
	"os"

	"golang.org/x/sys/unix"
)

// closable accepts only the games of the user running Team Follow.
func (darwinPlatform) closable(g game) bool {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", g.pid)
	return err == nil && int(info.Proc.P_pid) == g.pid && int(info.Eproc.Ucred.Uid) == os.Getuid()
}
