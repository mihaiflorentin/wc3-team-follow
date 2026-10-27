//go:build linux

package gui

import (
	"strings"

	"github.com/godbus/dbus/v5"
)

// alreadyOpen reports whether another Team Follow window holds the
// single-instance lock. It mirrors the D-Bus name Wails derives from
// uniqueID; Wails then hands the launch to that window and exits with
// code 1, which the crash reporter must not mistake for a crash.
func alreadyOpen(uniqueID string) bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	id := "wails_app_" + strings.NewReplacer("-", "_", ".", "_").Replace(uniqueID)
	var owned bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org."+id+".SingleInstance").Store(&owned)
	return err == nil && owned
}
