package crashreport

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RestartCode is the exit code of a window that installed an update and
// asks to be started again (ADR 0009). The supervisor starts the new
// version instead of reporting a crash.
const RestartCode = 75

// RestartEnv carries the process id of the instance that restarted for an
// update. The new instance waits for it to exit before it opens its window,
// so it doesn't meet the old one's single-instance lock.
const RestartEnv = "WF_RESTART_AFTER"

// UpdatedEnv marks a start that follows an update, for the window's
// after-update notes. WaitForPrevious sets it; supervised children inherit it.
const UpdatedEnv = "WF_UPDATED"

// Relaunch starts exe again as a new, independent instance that waits for
// this process to exit (RestartEnv). It is not marked as supervised, so on
// Windows and Linux it starts its own supervisor. On macOS an app bundle is
// opened through Launch Services (open -n), so the Dock shows it as usual.
func Relaunch(exe string) error {
	env := []string{RestartEnv + "=" + strconv.Itoa(os.Getpid())}
	var cmd *exec.Cmd
	if bundle, ok := appBundle(exe); ok && runtime.GOOS == "darwin" {
		cmd = exec.Command("/usr/bin/open", "-n", "-a", bundle, "--env", env[0])
	} else {
		cmd = exec.Command(exe)
		cmd.Env = append(cleanEnv(), env...)
		detach(cmd)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", exe, err)
	}
	if cmd.Path == "/usr/bin/open" {
		// open returns once Launch Services has the app started.
		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("open %s: %w", exe, err)
		}
		return nil
	}
	return cmd.Process.Release()
}

// appBundle returns the .app bundle that holds exe, at
// <bundle>.app/Contents/MacOS/<name>.
func appBundle(exe string) (string, bool) {
	macOS := filepath.Dir(exe)
	contents := filepath.Dir(macOS)
	bundle := filepath.Dir(contents)
	ok := filepath.Base(macOS) == "MacOS" && filepath.Base(contents) == "Contents" && strings.HasSuffix(bundle, ".app")
	return bundle, ok
}

// cleanEnv is this process's environment without the supervisor's and the
// restart's marks.
func cleanEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name != ChildEnv && name != RestartEnv && name != UpdatedEnv {
			env = append(env, kv)
		}
	}
	return env
}

// WaitForPrevious waits, at most timeout, for the instance that restarted
// for an update (RestartEnv) to exit, then marks this start as one after an
// update (Updated). Without RestartEnv it does nothing.
func WaitForPrevious(timeout time.Duration) {
	value := os.Getenv(RestartEnv)
	if value == "" {
		return
	}
	if pid, err := strconv.Atoi(value); err == nil && pid > 0 && pid != os.Getpid() {
		WaitForExit(pid, timeout)
	}
	_ = os.Unsetenv(RestartEnv)
	_ = os.Setenv(UpdatedEnv, "1")
}

// Updated reports whether this start follows an update.
func Updated() bool { return os.Getenv(UpdatedEnv) != "" }

// WebviewWait is how long the first start after an update waits for the
// old window's web view to let go of its data folder.
const WebviewWait = 15 * time.Second

// WaitUntilFree checks free every poll until it reports true or timeout
// has passed, and returns how long it waited and whether it got free. It
// waits for the WebView2 data folder after an update (WebviewFree): the
// old window's browser processes can outlive it, and WebView2 lets only one
// browser use a data folder at a time.
func WaitUntilFree(free func() bool, timeout, poll time.Duration) (time.Duration, bool) {
	if free() {
		return 0, true
	}
	start := time.Now()
	deadline := start.Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(min(poll, time.Until(deadline)))
		if free() {
			return time.Since(start), true
		}
	}
	return time.Since(start), false
}

// WaitForExit waits until the process pid has exited, at most timeout, and
// reports whether it did.
func WaitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if gone(pid, time.Until(deadline)) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}
