//go:build linux || darwin

package discovery

import (
	"errors"
	"fmt"
	"syscall"
)

// signalProcess asks pid to close with SIGTERM, or kills it with SIGKILL
// when force is set. A process that is already gone is not an error.
func signalProcess(pid int, force bool) error {
	if pid <= 0 {
		// 0 and -1 would signal a whole process group or every process.
		return fmt.Errorf("refusing to signal pid %d", pid)
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := syscall.Kill(pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
