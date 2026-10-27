//go:build !windows

package crashreport

import (
	"os"
	"os/signal"
	"syscall"
)

// forwardSignals passes stop requests sent to the supervisor on to the
// child, so stopping either process closes the window cleanly.
func forwardSignals(child *os.Process) (stop func()) {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-ch:
				_ = child.Signal(sig)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}

// stoppedOnRequest reports a child ended by a stop request (interrupt,
// terminate, hang-up or kill) rather than by an error.
func stoppedOnRequest(state *os.ProcessState) bool {
	ws, ok := state.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return false
	}
	switch ws.Signal() {
	case syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGKILL:
		return true
	}
	return false
}
