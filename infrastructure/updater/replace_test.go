package updater

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWindowsMovesTheRunningExeAsideAndCleansItUpNextStart(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "wc3-team-follow.exe")
	write(t, exe, "old", 0o755)
	unpacked := filepath.Join(dir, ".wc3-team-follow-update-1", "new")
	write(t, filepath.Join(unpacked, "wc3-team-follow-windows-amd64.exe"), "new", 0o755)

	if err := replaceExe(exe, unpacked); err != nil {
		t.Fatal(err)
	}
	if read(t, exe) != "new" || read(t, exe+".old") != "old" {
		t.Fatalf("exe %q, old %q", read(t, exe), read(t, exe+".old"))
	}

	if err := cleanup(target{path: exe, dir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatal(".old left behind")
	}
	if _, err := os.Stat(filepath.Join(dir, ".wc3-team-follow-update-1")); !os.IsNotExist(err) {
		t.Fatal("temporary folder left behind")
	}
	if read(t, exe) != "new" {
		t.Fatal("cleanup touched the program")
	}
}

func TestWindowsPutsTheOldExeBackWhenTheNewOneIsMissing(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "wc3-team-follow.exe")
	write(t, exe, "old", 0o755)
	unpacked := filepath.Join(dir, "new")
	write(t, filepath.Join(unpacked, "readme.txt"), "no program", 0o644)
	if err := replaceExe(exe, unpacked); err == nil {
		t.Fatal("installed without a program")
	}
	if read(t, exe) != "old" {
		t.Fatal("the old program was lost")
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatal(".old created for nothing")
	}
}

func TestMacOSSwapsTheWholeBundle(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Warcraft Team Follow.app")
	write(t, filepath.Join(bundle, "Contents/MacOS/wc3-team-follow"), "old", 0o755)
	write(t, filepath.Join(bundle, "Contents/Resources/stale.icns"), "old icon", 0o644)
	staging := filepath.Join(dir, ".wc3-team-follow-update-1")
	unpacked := filepath.Join(staging, "new")
	write(t, filepath.Join(unpacked, "Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow"), "new", 0o755)
	write(t, filepath.Join(unpacked, "Warcraft Team Follow.app/Contents/Info.plist"), "0.8.0", 0o644)

	if err := swapBundle(bundle, unpacked, staging); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(bundle, "Contents/MacOS/wc3-team-follow")) != "new" || read(t, filepath.Join(bundle, "Contents/Info.plist")) != "0.8.0" {
		t.Fatal("bundle not replaced")
	}
	if _, err := os.Stat(filepath.Join(bundle, "Contents/Resources/stale.icns")); !os.IsNotExist(err) {
		t.Fatal("old bundle's files mixed into the new one")
	}
	if info, err := os.Stat(filepath.Join(bundle, "Contents/MacOS/wc3-team-follow")); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o755) {
		t.Fatalf("program mode %v, %v", info, err)
	}
}

func TestMacOSRefusesATranslocatedApp(t *testing.T) {
	exe := "/private/var/folders/xy/T/AppTranslocation/1234-ABCD/d/Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow"
	_, err := locateBundle(exe)
	var p contract.UpdateProblem
	if !errors.As(err, &p) || !strings.Contains(p.Problem(), "Applications") {
		t.Fatalf("err = %v", err)
	}
	if _, err := locateBundle("/usr/local/bin/wc3-team-follow"); err == nil {
		t.Fatal("found a bundle around a plain program")
	}
	tgt, err := locateBundle("/Applications/Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow")
	if err != nil || tgt.path != filepath.FromSlash("/Applications/Warcraft Team Follow.app") || tgt.dir != filepath.FromSlash("/Applications") {
		t.Fatalf("target = %+v, %v", tgt, err)
	}
}

func TestMacOSRefusesAFolderItCannotWriteTo(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a read-only folder")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := makeStaging(target{path: filepath.Join(dir, "Warcraft Team Follow.app"), dir: dir}, "darwin")
	var p contract.UpdateProblem
	if !errors.As(err, &p) || !strings.Contains(p.Problem(), "Applications") {
		t.Fatalf("err = %v", err)
	}
	_, err = makeStaging(target{path: filepath.Join(dir, "wc3-team-follow"), dir: dir}, "linux")
	if !errors.As(err, &p) || !strings.Contains(p.Problem(), "can't write") {
		t.Fatalf("err = %v", err)
	}
}

func TestLinuxReplacesTheBinaryKeepingItsMode(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "wc3-team-follow")
	write(t, exe, "old", 0o750)
	unpacked := filepath.Join(dir, "new")
	write(t, filepath.Join(unpacked, "wc3-team-follow"), "new", 0o644)
	write(t, filepath.Join(unpacked, "wc3-team-follow.desktop"), "[Desktop Entry]", 0o644)
	if err := replaceBinary(exe, unpacked); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(exe)
	if err != nil || read(t, exe) != "new" || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o750) {
		t.Fatalf("binary %q mode %v, %v", read(t, exe), info.Mode(), err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("left behind %v", entries)
	}
}
