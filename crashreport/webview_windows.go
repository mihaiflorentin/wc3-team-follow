//go:build windows

package crashreport

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// WebviewFree reports whether no WebView2 browser holds the data folder
// dir. A running browser keeps EBWebView\lockfile open without sharing (and
// deletes it on close), so the folder is free when that file is missing or
// can be opened exclusively. Any other error counts as free, so a check
// that can't tell never holds the window back.
func WebviewFree(dir string) bool {
	path, err := windows.UTF16PtrFromString(filepath.Join(dir, "EBWebView", "lockfile"))
	if err != nil {
		return true
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err == nil {
		_ = windows.CloseHandle(h)
		return true
	}
	return !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
