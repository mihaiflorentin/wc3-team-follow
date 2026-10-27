//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

var procAttachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

// attachConsole lets commands such as `discover` print to the terminal
// that started them: the release is a GUI program with no console of its
// own. Without arguments the window opens and nothing is attached.
func attachConsole() {
	if len(os.Args) < 2 {
		return
	}
	const attachParentProcess = ^uintptr(0)
	if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
		return
	}
	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = out, out
	}
}
