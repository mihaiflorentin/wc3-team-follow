//go:build !windows

package crashreport

// WebviewFree is only needed on Windows, where WebView2 lets one browser
// use a data folder at a time; elsewhere the folder is always free.
func WebviewFree(string) bool { return true }
