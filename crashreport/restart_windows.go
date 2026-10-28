//go:build windows

package crashreport

import (
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// detach starts cmd in its own process group, so it outlives this process.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// gone waits up to wait for the process pid to exit and reports whether it
// has. A process that can't be opened no longer exists.
func gone(pid int, wait time.Duration) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true
	}
	defer func() { _ = windows.CloseHandle(h) }()
	ms := uint32(max(wait.Milliseconds(), 0))
	event, err := windows.WaitForSingleObject(h, ms)
	return err == nil && event == windows.WAIT_OBJECT_0
}
