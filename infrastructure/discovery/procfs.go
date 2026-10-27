package discovery

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// procfs discovers a game that Wine or Proton runs on Linux, using /proc.
//
// Under Wine the game is an ordinary Linux process whose argv[0] is the
// Windows path of Warcraft III.exe. Wine's wineserver may own the menu's
// listening socket, so every loopback listener of the game's user is a
// candidate; the GlueManager probe confirms the right one. Proton runs the
// game inside Steam's container, so files are read through
// /proc/<pid>/root.
type procfs struct {
	root string
	// home is the user's home folder, for the default ~/.wine prefix.
	home string
}

// loaders are argv[0] names that run the program given in the next
// arguments: Wine's loaders, and the x86 emulators people run Wine under on
// ARM machines (box64, FEX, qemu, Rosetta), which may stack two deep.
var loaders = []string{
	"wine", "wine64", "wine-preloader", "wine64-preloader", "wineloader",
	"box64", "FEXInterpreter", "FEXLoader", "qemu-x86_64", "qemu-x86_64-static", "rosetta",
}

func (p procfs) path(parts ...string) string {
	return filepath.Join(append([]string{p.root}, parts...)...)
}

func (p procfs) pids() []int {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

func (p procfs) cmdline(pid int) []string {
	raw, err := os.ReadFile(p.path(strconv.Itoa(pid), "cmdline"))
	if err != nil || len(raw) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
}

// gameArg returns the Windows path of the game executable when argv runs
// it, directly or through up to three loaders (for example an emulator
// running wine64-preloader).
func gameArg(argv []string) (string, bool) {
	for i, arg := range argv {
		if isGameExe(arg) {
			return arg, true
		}
		if i == 3 || !slices.Contains(loaders, baseName(arg)) && (i == 0 || !isGameBinaryForLoader(arg)) {
			return "", false
		}
	}
	return "", false
}

// isGameBinaryForLoader accepts the Unix path of the program a loader runs
// (an emulator's argv[1] is the Linux binary, followed by the Windows argv).
func isGameBinaryForLoader(arg string) bool {
	return strings.HasPrefix(arg, "/") && !strings.Contains(arg, `\`)
}

func (p procfs) findGame(context.Context) (game, error) {
	var found []game
	for _, pid := range p.pids() {
		if _, ok := gameArg(p.cmdline(pid)); ok {
			found = append(found, game{pid: pid, uid: p.uid(pid)})
		}
	}
	switch len(found) {
	case 0:
		return game{}, ErrNoGame
	case 1:
		return found[0], nil
	default:
		return game{}, ErrManyGames
	}
}

func (p procfs) uid(pid int) int {
	raw, err := os.ReadFile(p.path(strconv.Itoa(pid), "status"))
	if err != nil {
		return -1
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			if fields := strings.Fields(rest); len(fields) > 0 {
				if uid, err := strconv.Atoi(fields[0]); err == nil {
					return uid
				}
			}
		}
	}
	return -1
}

// socketInodes returns the inodes of the sockets pid holds open.
func (p procfs) socketInodes(pid int) map[string]bool {
	dir := p.path(strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	inodes := map[string]bool{}
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if inode, ok := strings.CutPrefix(target, "socket:["); ok {
			inodes[strings.TrimSuffix(inode, "]")] = true
		}
	}
	return inodes
}

type tcpListener struct {
	port  int
	uid   int
	inode string
}

// loopbackListeners parses /proc/<pid>/net/tcp (the game's network
// namespace) for sockets listening on 127.0.0.1.
func (p procfs) loopbackListeners(pid int) ([]tcpListener, error) {
	raw, err := os.ReadFile(p.path(strconv.Itoa(pid), "net", "tcp"))
	if err != nil {
		return nil, err
	}
	var out []tcpListener
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Scan() // header
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		// sl local rem st tx:rx tr:when retrnsmt uid timeout inode
		if len(f) < 10 || f[3] != "0A" {
			continue
		}
		addr, portHex, ok := strings.Cut(f[1], ":")
		if !ok || addr != "0100007F" {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil {
			continue
		}
		uid, _ := strconv.Atoi(f[7])
		out = append(out, tcpListener{port: int(port), uid: uid, inode: f[9]})
	}
	return out, nil
}

// listeners returns the game's own loopback listeners first, then the
// other listeners of the same user (Wine's wineserver may hold the socket).
func (p procfs) listeners(_ context.Context, g game) ([]int, error) {
	all, err := p.loopbackListeners(g.pid)
	if err != nil {
		return nil, err
	}
	own := p.socketInodes(g.pid)
	var first, rest []int
	for _, l := range all {
		switch {
		case own[l.inode]:
			first = append(first, l.port)
		case l.uid == g.uid && l.port >= 1024:
			rest = append(rest, l.port)
		}
	}
	slices.Sort(first)
	slices.Sort(rest)
	return slices.Compact(append(first, rest...)), nil
}

// session searches the game's private writable memory through
// /proc/<pid>/mem. Linux allows that only when ptrace is permitted: with
// Yama's ptrace_scope=1 (Ubuntu's default) the app needs CAP_SYS_PTRACE.
func (p procfs) session(ctx context.Context, g game) (string, error) {
	regions, err := p.privateRegions(g.pid)
	if err != nil {
		return "", accessError(err)
	}
	mem, err := os.Open(p.path(strconv.Itoa(g.pid), "mem"))
	if err != nil {
		return "", accessError(err)
	}
	defer func() { _ = mem.Close() }()

	var denied error
	session := scanRegions(ctx, regions, func(buf []byte, addr uint64) (int, error) {
		n, err := mem.ReadAt(buf, int64(addr))
		if n == 0 && err != nil && denied == nil && (errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "input/output error")) {
			denied = err
		}
		return n, err
	})
	if session != "" {
		return session, nil
	}
	if denied != nil {
		return "", accessError(denied)
	}
	return "", ErrNoSession
}

func accessError(err error) error {
	if errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "input/output error") {
		return fmt.Errorf("%w To allow it, run: sudo setcap cap_sys_ptrace=ep <path to wc3-team-follow>", ErrAccess)
	}
	return fmt.Errorf("read game memory: %w", err)
}

// privateRegions returns the anonymous private read-write mappings: the
// heap memory Windows reports as MEM_PRIVATE.
func (p procfs) privateRegions(pid int) ([]region, error) {
	raw, err := os.ReadFile(p.path(strconv.Itoa(pid), "maps"))
	if err != nil {
		return nil, err
	}
	var out []region
	for line := range strings.SplitSeq(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[1], "rw") || f[1][3] != 'p' {
			continue
		}
		if len(f) > 5 && f[5] != "[heap]" {
			continue
		}
		span := strings.SplitN(f[0], "-", 2)
		if len(span) != 2 {
			continue
		}
		start, err1 := strconv.ParseUint(span[0], 16, 64)
		end, err2 := strconv.ParseUint(span[1], 16, 64)
		if err1 == nil && err2 == nil && end > start {
			out = append(out, region{start, end})
		}
	}
	return out, nil
}

// version reads the game build from Warcraft III.exe, found either among
// the files Wine mapped into the process or by translating the Windows
// path through the Wine prefix.
func (p procfs) version(_ context.Context, g game) string {
	for _, candidate := range p.exeCandidates(g.pid) {
		if v := peFileVersion(candidate); v != "" {
			return v
		}
	}
	return ""
}

func (p procfs) exeCandidates(pid int) []string {
	id := strconv.Itoa(pid)
	// Files are opened through the game's root: Proton's container has its
	// own mount namespace.
	inGame := func(path string) string { return p.path(id, "root", path) }

	var out []string
	add := func(path string) {
		// Try the game's root first, then the path as this process sees it.
		for _, candidate := range []string{inGame(path), path} {
			if !slices.Contains(out, candidate) {
				out = append(out, candidate)
			}
		}
	}
	if raw, err := os.ReadFile(p.path(id, "maps")); err == nil {
		for line := range strings.SplitSeq(string(raw), "\n") {
			if i := strings.Index(line, "/"); i >= 0 {
				if path := strings.TrimSuffix(line[i:], " (deleted)"); isGameExe(path) {
					add(path)
				}
			}
		}
	}
	winPath, ok := gameArg(p.cmdline(pid))
	if !ok {
		return out
	}
	for _, prefix := range p.winePrefixes(pid) {
		if path, ok := dosPath(prefix, winPath); ok {
			add(path)
		}
	}
	return out
}

// winePrefixes lists the prefix folders the game may run in.
func (p procfs) winePrefixes(pid int) []string {
	env := p.environ(pid)
	var out []string
	if v := env["WINEPREFIX"]; v != "" {
		out = append(out, v)
	}
	if v := env["STEAM_COMPAT_DATA_PATH"]; v != "" {
		out = append(out, filepath.Join(v, "pfx"))
	}
	home := env["HOME"]
	if home == "" {
		home = p.home
	}
	if home != "" {
		out = append(out, filepath.Join(home, ".wine"))
	}
	return out
}

func (p procfs) environ(pid int) map[string]string {
	raw, err := os.ReadFile(p.path(strconv.Itoa(pid), "environ"))
	if err != nil {
		return nil
	}
	env := map[string]string{}
	for entry := range strings.SplitSeq(string(raw), "\x00") {
		if k, v, ok := strings.Cut(entry, "="); ok {
			env[k] = v
		}
	}
	return env
}

// dosPath maps a Windows path such as C:\Games\x.exe to
// <prefix>/dosdevices/c:/Games/x.exe.
func dosPath(prefix, winPath string) (string, bool) {
	if len(winPath) < 3 || winPath[1] != ':' || (winPath[2] != '\\' && winPath[2] != '/') {
		return "", false
	}
	drive := strings.ToLower(winPath[:2])
	rest := strings.ReplaceAll(winPath[3:], `\`, "/")
	return filepath.Join(prefix, "dosdevices", drive, rest), true
}
