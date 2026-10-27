package discovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.FromSlash(d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func folderPaths(p procfs, g *game) []string {
	var out []string
	for _, f := range p.mapsFolders(context.Background(), g) {
		out = append(out, f.Path)
	}
	return out
}

func TestLinuxPrefixesFromTheRunningGame(t *testing.T) {
	p, _ := fakeProc(t)
	p.home = ""
	// The fake game runs under Proton (STEAM_COMPAT_DATA_PATH) and its exe
	// is mapped from inside the prefix.
	var dirs []string
	for _, pf := range p.prefixes(&game{pid: 4242}) {
		dirs = append(dirs, pf.dir)
	}
	if !slices.Equal(dirs, []string{"/steam/compatdata/123/pfx"}) {
		t.Fatalf("prefixes = %v", dirs)
	}
	if got := folderPaths(p, nil); len(got) != 0 {
		t.Fatalf("folders without a game or home: %v", got)
	}
}

func TestLinuxMapsFoldersScanSteamLutrisBottlesAndWine(t *testing.T) {
	home := filepath.ToSlash(t.TempDir())
	p := procfs{root: filepath.ToSlash(t.TempDir()), home: home}
	steamLib := filepath.ToSlash(t.TempDir())
	mkdirs(t,
		home+"/.wine/drive_c/users/me/Documents",
		home+"/.local/share/Steam/steamapps/compatdata/2000/pfx/drive_c/users/steamuser/Documents",
		steamLib+"/steamapps/compatdata/3000/pfx/drive_c/users/steamuser/My Documents",
		home+"/Games/battlenet/drive_c/users/me/Documents",
		home+"/.var/app/com.usebottles.bottles/data/bottles/bottles/BNet/drive_c/users/me/Documents",
	)
	vdf := `"libraryfolders" { "0" { "path" "` + steamLib + `" } }`
	if err := os.WriteFile(filepath.FromSlash(home+"/.local/share/Steam/steamapps/libraryfolders.vdf"), []byte(vdf), 0o644); err != nil {
		t.Fatal(err)
	}
	got := folderPaths(p, nil)
	for _, want := range []string{
		home + "/.wine/drive_c/users/me/Documents/Warcraft III/Maps",
		home + "/.local/share/Steam/steamapps/compatdata/2000/pfx/drive_c/users/steamuser/Documents/Warcraft III/Maps",
		steamLib + "/steamapps/compatdata/3000/pfx/drive_c/users/steamuser/My Documents/Warcraft III/Maps",
		home + "/Games/battlenet/drive_c/users/me/Documents/Warcraft III/Maps",
		home + "/.var/app/com.usebottles.bottles/data/bottles/bottles/BNet/drive_c/users/me/Documents/Warcraft III/Maps",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestLinuxRunningGamePrefixComesFirst(t *testing.T) {
	home := filepath.ToSlash(t.TempDir())
	pfx := filepath.ToSlash(t.TempDir())
	mkdirs(t, home+"/.wine/drive_c/users/me/Documents", pfx+"/drive_c/users/steamuser/Documents")
	root := filepath.ToSlash(t.TempDir())
	mkdirs(t, root+"/77")
	if err := os.WriteFile(filepath.FromSlash(root+"/77/environ"), []byte("WINEPREFIX="+pfx+"\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := procfs{root: root, home: home}
	got := folderPaths(p, &game{pid: 77})
	if len(got) < 2 || got[0] != pfx+"/drive_c/users/steamuser/Documents/Warcraft III/Maps" {
		t.Fatalf("got %v", got)
	}
}

func TestLinuxGamePathUnderDriveC(t *testing.T) {
	home := filepath.ToSlash(t.TempDir())
	mkdirs(t, home+"/.wine/drive_c/users/me")
	p := procfs{root: filepath.ToSlash(t.TempDir()), home: home}
	got := p.gamePath(context.Background(), nil, home+"/.wine/drive_c/users/me/Documents/Warcraft III/Maps/Download/Legion TD.w3x")
	if want := `C:\users\me\Documents\Warcraft III\Maps\Download\Legion TD.w3x`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLinuxGamePathThroughDosDevices(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	home := t.TempDir()
	maps := t.TempDir()
	mkdirs(t, home+"/.wine/dosdevices", home+"/.wine/drive_c")
	_ = os.Symlink("/", home+"/.wine/dosdevices/z:")
	_ = os.Symlink(maps, home+"/.wine/dosdevices/d:")
	p := procfs{root: t.TempDir(), home: home}
	if got := p.gamePath(context.Background(), nil, maps+"/x.w3x"); got != `D:\x.w3x` {
		t.Errorf("D: got %q", got)
	}
	if got := p.gamePath(context.Background(), nil, "/srv/maps/y.w3x"); got != `Z:\srv\maps\y.w3x` {
		t.Errorf("Z: got %q", got)
	}
}

func TestLinuxGamePathFallsBackToZ(t *testing.T) {
	p := procfs{root: filepath.ToSlash(t.TempDir()), home: ""}
	if got := p.gamePath(context.Background(), nil, "/home/me/maps/z.w3m"); got != `Z:\home\me\maps\z.w3m` {
		t.Fatalf("got %q", got)
	}
}
