//go:build windows

package crashreport

import "golang.org/x/sys/windows"

// Supervises is true where a supervisor process runs the window.
const Supervises = true

// notOpenedHint explains the usual cause of a window that never opens.
const notOpenedHint = "The window needs the Microsoft Edge WebView2 Runtime. Repair or reinstall it (Settings > Apps > Installed apps > Microsoft Edge WebView2 Runtime > Modify > Repair, or download it from https://developer.microsoft.com/microsoft-edge/webview2/), restart Windows, then start Team Follow again."

// Show displays an error message box.
func Show(title, message string) {
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	m, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, m, t, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND|windows.MB_TOPMOST)
}
