//go:build !windows && !darwin

package crashreport

import (
	"fmt"
	"os"
	"os/exec"
)

// Supervises is true where a supervisor process runs the window.
const Supervises = true

// notOpenedHint explains the usual cause of a window that never opens.
const notOpenedHint = "The window needs WebKitGTK 4.1 (libwebkit2gtk-4.1) and a running desktop session. Install it with your package manager, then start Team Follow again."

// Show prints the message and displays it with the first dialog tool the
// desktop has: zenity (GNOME and most others), kdialog (KDE), or xmessage.
// Without any of them, the printed message is all there is.
func Show(title, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
	for _, tool := range [][]string{
		{"zenity", "--error", "--no-markup", "--title=" + title, "--text=" + message},
		{"kdialog", "--title", title, "--error", message},
		{"xmessage", "-center", title + "\n\n" + message},
	} {
		if path, err := exec.LookPath(tool[0]); err == nil {
			if exec.Command(path, tool[1:]...).Run() == nil {
				return
			}
		}
	}
}
