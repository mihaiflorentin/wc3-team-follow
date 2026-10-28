// Package crashreport tells the player when Team Follow stops because of an
// error. On Windows and Linux a parent process runs the window as a child
// and shows a dialog when the child fails, because Wails and WebView2 end
// the process themselves (os.Exit, log.Fatal) where no recover reaches. On
// macOS, where re-launching the app bundle would confuse the Dock, the
// window process shows the dialog itself.
package crashreport

import (
	"fmt"
	"regexp"
	"strings"
)

// Title is the error dialog's title.
const Title = "Team Follow stopped"

// maxReason caps the reason shown in the dialog.
const maxReason = 400

// StartupMarker is logged once the window has started. A child that exits
// with code 1 before it, saying nothing, is a second launch that Wails on
// Linux turned away, not a crash.
const StartupMarker = "gui.startup"

// AlreadyOpenMarker is logged when another window already holds the
// single-instance lock. Wails on Linux runs the startup hook before it
// checks the lock, so a second launch logs StartupMarker too and then exits
// with code 1 after handing over to the open window.
const AlreadyOpenMarker = "gui.already_open"

var (
	ansi   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	stdlog = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} (.+)$`)
	// sigLine is how Go reports a crash by signal: "SIGSEGV: segmentation violation".
	sigLine = regexp.MustCompile(`^SIG[A-Z]+: `)
)

// Report decides whether the child's exit needs a dialog and writes its
// message. output is the end of what the child printed; quiet marks an exit
// the player asked for (the process was told to stop).
func Report(output string, code int, quiet bool, logDir string) (string, bool) {
	if code == 0 || quiet {
		return "", false
	}
	output = ansi.ReplaceAllString(output, "")
	reason := failureLine(output)
	if reason == "" {
		if code == 1 && (!strings.Contains(output, StartupMarker) || strings.Contains(output, AlreadyOpenMarker)) {
			return "", false
		}
		reason = fmt.Sprintf("It exited with code %d.", code)
	}
	return Message(reason, logDir), true
}

// NotOpenedMessage is the dialog text for a window that never started.
func NotOpenedMessage(logDir string) string {
	msg := "The Team Follow window didn't open, so Team Follow will close.\n\n" + notOpenedHint
	if logDir != "" {
		msg += "\n\nThe log files are in " + logDir + ". Attach them when you report the problem."
	}
	return msg
}

// UpdatedNotStartedMessage is the dialog text when the updated version
// couldn't be started after an update.
func UpdatedNotStartedMessage(err error) string {
	return "Team Follow was updated, but the new version couldn't be started: " + err.Error() + "\n\nOpen Team Follow again."
}

// Message is the dialog text for reason.
func Message(reason, logDir string) string {
	if r := []rune(reason); len(r) > maxReason {
		reason = string(r[:maxReason]) + "…"
	}
	msg := "Team Follow stopped because of an error:\n\n" + reason
	if logDir != "" {
		msg += "\n\nThe log files are in " + logDir + ". Attach them when you report the problem."
	}
	return msg
}

// failureLine finds the line that best explains the failure: a Go panic,
// fatal error or crash by signal, a WebView2 error, the app's own "error:"
// line, or a line from the standard logger (what log.Fatal prints).
func failureLine(output string) string {
	lines := strings.Split(output, "\n")
	for _, match := range []func(string) (string, bool){
		func(l string) (string, bool) {
			return l, strings.HasPrefix(l, "panic: ") || strings.HasPrefix(l, "fatal error: ") || sigLine.MatchString(l)
		},
		func(l string) (string, bool) { return l, strings.HasPrefix(l, "[WebView2 Error]") },
		func(l string) (string, bool) {
			rest, ok := strings.CutPrefix(l, "error: ")
			return rest, ok
		},
		func(l string) (string, bool) {
			if m := stdlog.FindStringSubmatch(l); m != nil {
				return m[1], true
			}
			return "", false
		},
	} {
		for _, l := range lines {
			if reason, ok := match(strings.TrimSpace(l)); ok && reason != "" {
				return reason
			}
		}
	}
	return ""
}
