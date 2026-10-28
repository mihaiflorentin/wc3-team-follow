//go:build windows

package crashreport

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWebviewFolderIsHeldWhileItsLockfileIsOpen(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "EBWebView"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, _ := windows.UTF16PtrFromString(filepath.Join(dir, "EBWebView", "lockfile"))
	// As Chromium holds it: open for writing, shared for reading only,
	// deleted on close.
	h, err := windows.CreateFile(path, windows.GENERIC_WRITE, windows.FILE_SHARE_READ, nil, windows.CREATE_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
	if err != nil {
		t.Fatal(err)
	}
	if WebviewFree(dir) {
		t.Fatal("a held data folder counts as free")
	}
	_ = windows.CloseHandle(h)
	if !WebviewFree(dir) {
		t.Fatal("a released data folder counts as held")
	}
}
