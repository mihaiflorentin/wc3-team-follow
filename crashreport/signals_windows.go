//go:build windows

package crashreport

import "os"

// forwardSignals has nothing to do on Windows: closing the window ends the
// child, and the child's exit ends the supervisor.
func forwardSignals(*os.Process) (stop func()) { return func() {} }

// stoppedOnRequest is false on Windows: a terminated process exits with a
// code, and a non-zero code is reported.
func stoppedOnRequest(*os.ProcessState) bool { return false }
