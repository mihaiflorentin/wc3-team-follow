package crashreport

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSuperviseRelaunchesOnAnUpdateRestartWithoutADialog(t *testing.T) {
	t.Setenv(helperEnv, "restart")
	exe, _ := os.Executable()
	var shown []string
	relaunched := 0
	code := Supervise(Options{Exe: exe, StartupTimeout: time.Minute, Show: func(_, msg string) { shown = append(shown, msg) },
		Relaunch: func() error { relaunched++; return nil }})
	if code != 0 || len(shown) != 0 || relaunched != 1 {
		t.Fatalf("code %d, shown %q, relaunched %d", code, shown, relaunched)
	}
}

func TestSuperviseSaysSoWhenTheUpdatedAppCannotStart(t *testing.T) {
	t.Setenv(helperEnv, "restart")
	exe, _ := os.Executable()
	var shown []string
	code := Supervise(Options{Exe: exe, Show: func(_, msg string) { shown = append(shown, msg) },
		Relaunch: func() error { return errors.New("exec format error") }})
	if code != 1 || len(shown) != 1 || !strings.Contains(shown[0], "was updated") {
		t.Fatalf("code %d, shown %q", code, shown)
	}
}

func TestRelaunchStartsTheNewInstanceToWaitForThisOne(t *testing.T) {
	out := t.TempDir() + "/out"
	t.Setenv(helperEnv, "relaunched")
	t.Setenv("CRASHREPORT_OUT", out)
	t.Setenv(ChildEnv, "1")
	exe, _ := os.Executable()
	if err := Relaunch(exe); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(out); err == nil && len(data) > 0 {
			// The new instance waits for this process, and isn't marked as
			// supervised: it starts its own supervisor.
			if got, want := string(data), strconv.Itoa(os.Getpid())+"|"; got != want {
				t.Fatalf("new instance saw %q, want %q", got, want)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the new instance didn't start")
}

func TestWaitForExitWaitsUntilTheOldProcessIsGone(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"=linger")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	started := time.Now()
	if !WaitForExit(cmd.Process.Pid, 10*time.Second) {
		t.Fatal("the old process didn't exit in time")
	}
	if waited := time.Since(started); waited < 200*time.Millisecond {
		t.Fatalf("returned after %v, before the old process exited", waited)
	}
}

func TestWaitForExitGivesUpAfterTheTimeout(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"=hang")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if WaitForExit(cmd.Process.Pid, 200*time.Millisecond) {
		t.Fatal("reported a running process as gone")
	}
}

func TestWaitForPreviousMarksAStartAfterAnUpdate(t *testing.T) {
	t.Setenv(RestartEnv, "")
	t.Setenv(UpdatedEnv, "")
	if WaitForPrevious(time.Second); Updated() {
		t.Fatal("a plain start counts as an update")
	}
	t.Setenv(RestartEnv, "999999999")
	WaitForPrevious(time.Second)
	if !Updated() || os.Getenv(RestartEnv) != "" {
		t.Fatalf("updated %v, restart env %q", Updated(), os.Getenv(RestartEnv))
	}
}

// superviseAfterUpdate supervises mode as the first start after an update
// and returns the exit code, the dialogs, the runs and the waits.
func superviseAfterUpdate(t *testing.T, mode string, retryWithin time.Duration) (int, []string, int, int) {
	t.Helper()
	out := t.TempDir() + "/runs"
	t.Setenv(helperEnv, mode)
	t.Setenv("CRASHREPORT_OUT", out)
	exe, _ := os.Executable()
	var shown []string
	waits := 0
	code := Supervise(Options{Exe: exe, Show: func(_, msg string) { shown = append(shown, msg) },
		RetryWithin: retryWithin, BeforeRetry: func() { waits++ }})
	data, _ := os.ReadFile(out)
	return code, shown, strings.Count(string(data), "run"), waits
}

func TestAfterAnUpdateAnEarlyFailureIsRetriedOnce(t *testing.T) {
	code, shown, runs, waits := superviseAfterUpdate(t, "flaky", 10*time.Second)
	if code != 0 || len(shown) != 0 || runs != 2 || waits != 1 {
		t.Fatalf("code %d, shown %q, runs %d, waits %d", code, shown, runs, waits)
	}
}

func TestAfterAnUpdateASecondFailureIsReported(t *testing.T) {
	code, shown, runs, waits := superviseAfterUpdate(t, "always-fails", 10*time.Second)
	if code != 1 || len(shown) != 1 || !strings.Contains(shown[0], "WebView2") || runs != 2 || waits != 1 {
		t.Fatalf("code %d, shown %q, runs %d, waits %d", code, shown, runs, waits)
	}
}

func TestWithoutAnUpdateAFailureIsNotRetried(t *testing.T) {
	code, shown, runs, waits := superviseAfterUpdate(t, "flaky", 0)
	if code != 1 || len(shown) != 1 || runs != 1 || waits != 0 {
		t.Fatalf("code %d, shown %q, runs %d, waits %d", code, shown, runs, waits)
	}
}

func TestALateFailureIsNotRetried(t *testing.T) {
	code, shown, runs, _ := superviseAfterUpdate(t, "flaky", time.Nanosecond)
	if code != 1 || len(shown) != 1 || runs != 1 {
		t.Fatalf("code %d, shown %q, runs %d", code, shown, runs)
	}
}

func TestWaitUntilFreeWaitsForTheDataFolder(t *testing.T) {
	checks := 0
	waited, free := WaitUntilFree(func() bool { checks++; return checks >= 3 }, 5*time.Second, 10*time.Millisecond)
	if !free || checks != 3 || waited < 20*time.Millisecond {
		t.Fatalf("free %v after %d checks, waited %v", free, checks, waited)
	}
	if waited, free := WaitUntilFree(func() bool { return true }, time.Second, time.Second); !free || waited != 0 {
		t.Fatalf("waited %v for a free folder", waited)
	}
}

func TestWaitUntilFreeGivesUp(t *testing.T) {
	started := time.Now()
	if _, free := WaitUntilFree(func() bool { return false }, 100*time.Millisecond, 10*time.Millisecond); free {
		t.Fatal("reported a held folder as free")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("waited past the timeout")
	}
}

func TestWebviewFolderWithoutALockIsFree(t *testing.T) {
	if !WebviewFree(t.TempDir()) {
		t.Fatal("an unused data folder counts as held")
	}
}
