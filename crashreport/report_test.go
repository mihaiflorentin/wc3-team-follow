package crashreport

import (
	"strings"
	"testing"
)

func TestReportPicksTheLineThatExplainsTheFailure(t *testing.T) {
	cases := map[string]struct {
		output string
		code   int
		want   string
	}{
		"app error":   {"\x1b[34mINFO\x1b[0m app.start\nerror: run window: no display\n", 1, "run window: no display"},
		"panic":       {"INFO gui.startup\npanic: runtime error: index out of range [3]\n\ngoroutine 1 [running]:\nerror: later\n", 2, "panic: runtime error: index out of range [3]"},
		"fatal error": {"fatal error: concurrent map writes\n", 2, "fatal error: concurrent map writes"},
		"signal":      {"INFO gui.startup\nSIGSEGV: segmentation violation\nPC=0x0 m=0 sigcode=0\n", 2, "SIGSEGV: segmentation violation"},
		"webview2":    {"ERR | WebVie2wProcess failed with kind 6\n[WebView2 Error] winapi error #2147947423\n\nStack trace:\n", 1, "[WebView2 Error] winapi error #2147947423"},
		"log.Fatal":   {"2026/10/27 22:13:29 Unable to create window\n", 1, "Unable to create window"},
		"unexplained": {"INFO -> gui.startup\n", 3, "It exited with code 3."},
	}
	for name, c := range cases {
		msg, ok := Report(c.output, c.code, false, "/logs")
		if !ok || !strings.Contains(msg, c.want) || !strings.Contains(msg, "/logs") {
			t.Errorf("%s: got %q, %v", name, msg, ok)
		}
	}
}

func TestReportStaysQuiet(t *testing.T) {
	cases := map[string]struct {
		output string
		code   int
		quiet  bool
	}{
		"closed normally":            {"INFO gui.startup\nINFO app.exit\n", 0, false},
		"stopped on request":         {"INFO gui.startup\n", -1, true},
		"second launch, turned away": {"INFO app.start\n", 1, false},
	}
	for name, c := range cases {
		if msg, ok := Report(c.output, c.code, c.quiet, ""); ok {
			t.Errorf("%s: reported %q", name, msg)
		}
	}
}

func TestReportAfterTheWindowStartedEvenWithoutAReason(t *testing.T) {
	if _, ok := Report("INFO -> gui.startup\n", 1, false, ""); !ok {
		t.Fatal("a silent exit after the window started was not reported")
	}
}

func TestMessageIsShortened(t *testing.T) {
	msg := Message(strings.Repeat("x", 1000), "")
	if strings.Count(msg, "x") != maxReason || strings.Contains(msg, "log files") {
		t.Fatalf("got %d x's: %q", strings.Count(msg, "x"), msg[:80])
	}
}
