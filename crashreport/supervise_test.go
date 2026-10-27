package crashreport

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const helperEnv = "CRASHREPORT_HELPER"

// TestMain lets the test binary act as the supervised child.
func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "ok":
		fmt.Println("INFO gui.startup")
		os.Exit(0)
	case "error":
		fmt.Fprintln(os.Stderr, "error: run window: no display")
		os.Exit(1)
	case "panic":
		fmt.Println("INFO gui.startup")
		panic("boom")
	case "second":
		os.Exit(1)
	case "hang":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "env":
		if !Supervised() {
			os.Exit(7)
		}
		os.Exit(0)
	}
	os.Exit(99)
}

func supervise(t *testing.T, mode string) (int, []string) {
	t.Helper()
	return superviseWithin(t, mode, time.Minute)
}

func superviseWithin(t *testing.T, mode string, timeout time.Duration) (int, []string) {
	t.Helper()
	t.Setenv(helperEnv, mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var shown []string
	code := Supervise(Options{Exe: exe, LogDir: "/logs", StartupTimeout: timeout, Show: func(title, msg string) { shown = append(shown, title+": "+msg) }})
	return code, shown
}

func TestSuperviseShowsNothingForACleanExit(t *testing.T) {
	if code, shown := supervise(t, "ok"); code != 0 || len(shown) != 0 {
		t.Fatalf("code %d, shown %q", code, shown)
	}
}

func TestSuperviseReportsTheAppError(t *testing.T) {
	code, shown := supervise(t, "error")
	if code != 1 || len(shown) != 1 || !strings.Contains(shown[0], "run window: no display") {
		t.Fatalf("code %d, shown %q", code, shown)
	}
}

func TestSuperviseReportsAPanic(t *testing.T) {
	code, shown := supervise(t, "panic")
	if code == 0 || len(shown) != 1 || !strings.Contains(shown[0], "panic: boom") {
		t.Fatalf("code %d, shown %q", code, shown)
	}
}

func TestSuperviseIgnoresATurnedAwaySecondLaunch(t *testing.T) {
	if code, shown := supervise(t, "second"); code != 1 || len(shown) != 0 {
		t.Fatalf("code %d, shown %q", code, shown)
	}
}

func TestSuperviseMarksTheChild(t *testing.T) {
	if code, _ := supervise(t, "env"); code != 0 {
		t.Fatalf("the child wasn't marked as supervised (code %d)", code)
	}
}

func TestSuperviseRecordsTheDialogInTheCrashFile(t *testing.T) {
	t.Setenv(helperEnv, "error")
	exe, _ := os.Executable()
	crash := t.TempDir() + "/crash.log"
	Supervise(Options{Exe: exe, CrashFile: crash, Show: func(string, string) {}})
	data, err := os.ReadFile(crash)
	if err != nil || !strings.Contains(string(data), "Team Follow stopped: Team Follow stopped because of an error:  run window: no display") {
		t.Fatalf("crash file %q, %v", data, err)
	}
}

func TestSuperviseReportsAChildThatCannotStart(t *testing.T) {
	var shown []string
	code := Supervise(Options{Exe: "/nonexistent/wc3-team-follow", Show: func(_, msg string) { shown = append(shown, msg) }})
	if code != 1 || len(shown) != 1 || !strings.Contains(shown[0], "couldn't start") {
		t.Fatalf("code %d, shown %q", code, shown)
	}
}

func TestSuperviseClosesAWindowThatNeverStarts(t *testing.T) {
	started := time.Now()
	code, shown := superviseWithin(t, "hang", 300*time.Millisecond)
	if time.Since(started) > 10*time.Second {
		t.Fatal("the stuck child wasn't closed")
	}
	if code == 0 || len(shown) != 1 || !strings.Contains(shown[0], "didn't open") {
		t.Fatalf("code %d, shown %q", code, shown)
	}
}
