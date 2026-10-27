//go:build !linux

package gui

// alreadyOpen is only needed on Linux: elsewhere Wails exits a second
// launch with code 0.
func alreadyOpen(string) bool { return false }
