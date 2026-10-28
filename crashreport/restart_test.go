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
