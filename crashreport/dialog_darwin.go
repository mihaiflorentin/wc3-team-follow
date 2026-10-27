//go:build darwin

package crashreport

import (
	"fmt"
	"os"
	"os/exec"
)

// Supervises is false on macOS: re-launching the app bundle's binary as a
// child would give it a second Dock icon. The window process shows the
// dialog itself.
const Supervises = false

// notOpenedHint explains the usual cause of a window that never opens.
const notOpenedHint = "Restart your Mac, then start Team Follow again."

// Show displays an alert through osascript, and prints the message.
func Show(title, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
	_ = exec.Command("/usr/bin/osascript",
		"-e", "on run argv",
		"-e", "display alert (item 1 of argv) message (item 2 of argv) as critical",
		"-e", "end run",
		title, message).Run()
}
