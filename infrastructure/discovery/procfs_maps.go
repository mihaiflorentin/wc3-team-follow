package discovery

import (
	"context"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// prefix is a Wine prefix (the folder holding drive_c) and how it was found.
type prefix struct {
	dir    string
	source string
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// prefixes lists the Wine prefixes the game may run in: the running game's
// own first (from its environment, or from its executable's path, which
// covers Steam → Proton → Battle.net → Warcraft III), then the usual
// places of Wine, Steam's Proton, Lutris and Bottles.
func (p procfs) prefixes(g *game) []prefix {
	var out []prefix
	seen := map[string]bool{}
	add := func(dir, source string) {
		dir = path.Clean(dir)
		if dir == "." || dir == "/" || seen[dir] {
			return
		}
		seen[dir] = true
		out = append(out, prefix{dir, source})
	}
	if g != nil {
		env := p.environ(g.pid)
		if v := env["WINEPREFIX"]; v != "" {
			add(v, "Prefix of the running game")
		}
		if v := env["STEAM_COMPAT_DATA_PATH"]; v != "" {
			add(path.Join(v, "pfx"), "Proton prefix of the running game")
		}
		for _, exe := range p.exeCandidates(g.pid) {
			if i := strings.Index(exe, "/drive_c/"); i > 0 {
				add(strings.TrimPrefix(exe[:i], p.path(strconv.Itoa(g.pid), "root")), "Prefix of the running game")
			}
		}
	}
	if p.home == "" {
		return out
	}
	add(path.Join(p.home, ".wine"), "Wine")
	for _, steam := range []string{
		path.Join(p.home, ".steam", "steam"),
		path.Join(p.home, ".local", "share", "Steam"),
		path.Join(p.home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
	} {
		libraries := []string{steam}
		if raw, err := os.ReadFile(path.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, m := range vdfPath.FindAllStringSubmatch(string(raw), -1) {
				libraries = append(libraries, m[1])
			}
		}
		for _, lib := range libraries {
			for _, game := range subdirs(path.Join(lib, "steamapps", "compatdata")) {
				add(path.Join(game, "pfx"), "Steam Proton")
			}
		}
	}
	for _, dir := range subdirs(path.Join(p.home, "Games")) {
		add(dir, "Lutris")
	}
	for _, bottles := range []string{
		path.Join(p.home, ".local", "share", "bottles", "bottles"),
		path.Join(p.home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "bottles"),
	} {
		for _, dir := range subdirs(bottles) {
			add(dir, "Bottles")
		}
	}
	return out
}

func subdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, path.Join(dir, e.Name()))
		}
	}
	return out
}

// mapsFolders looks for <prefix>/drive_c/users/<user>/Documents/Warcraft
// III/Maps in every prefix. Proton's user is "steamuser"; Wine uses the
// Linux user name, and may link Documents to ~/Documents.
func (p procfs) mapsFolders(_ context.Context, g *game) []dto.MapsFolder {
	var out []dto.MapsFolder
	for _, pf := range p.prefixes(g) {
		for _, user := range subdirs(path.Join(pf.dir, "drive_c", "users")) {
			for _, docs := range []string{"Documents", "My Documents"} {
				out = append(out, dto.MapsFolder{Path: path.Join(user, docs, "Warcraft III", "Maps"), Source: pf.source})
			}
		}
	}
	if p.home != "" {
		out = append(out, dto.MapsFolder{Path: path.Join(p.home, "Documents", "Warcraft III", "Maps"), Source: "Documents"})
	}
	return out
}

// gamePath turns a Linux path into the Windows path the game sees: under a
// prefix's drive_c it's on C:, otherwise on the drive letter Wine maps to
// its folder (Z: maps to / by default).
func (p procfs) gamePath(_ context.Context, g *game, host string) string {
	host = path.Clean(host)
	prefixes := p.prefixes(g)
	for _, pf := range prefixes {
		if rel, ok := under(host, path.Join(pf.dir, "drive_c")); ok {
			return `C:\` + windowsSlashes(rel)
		}
	}
	best, bestLen := "", -1
	for _, pf := range prefixes {
		devices := path.Join(pf.dir, "dosdevices")
		entries, err := os.ReadDir(devices)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if len(name) != 2 || name[1] != ':' {
				continue
			}
			target, err := os.Readlink(path.Join(devices, name))
			if err != nil {
				continue
			}
			if !path.IsAbs(target) {
				target = path.Join(devices, target)
			}
			if rel, ok := under(host, path.Clean(target)); ok && len(target) > bestLen {
				best, bestLen = strings.ToUpper(name)+`\`+windowsSlashes(rel), len(target)
			}
		}
	}
	if best != "" {
		return best
	}
	return `Z:` + windowsSlashes(host)
}

// under reports whether p is base or inside it, and the rest of the path.
func under(p, base string) (string, bool) {
	if p == base {
		return "", true
	}
	if base == "/" {
		return strings.TrimPrefix(p, "/"), strings.HasPrefix(p, "/")
	}
	if rest, ok := strings.CutPrefix(p, base+"/"); ok {
		return rest, true
	}
	return "", false
}

func windowsSlashes(p string) string { return strings.ReplaceAll(p, "/", `\`) }
