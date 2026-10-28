//go:build !windows

package crashreport

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// detach starts cmd in its own session, so it outlives this process and
// the terminal that started it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// gone reports whether no process pid exists any more. A process of another
// user (EPERM) still exists.
func gone(pid int, _ time.Duration) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
