package discovery

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const session = "123456789012345"

func TestSearchSessionFindsBothURLForms(t *testing.T) {
	for _, text := range []string{
		"xx/webui/index.html?guid=" + session + "&lang=en",
		"ws://127.0.0.1:5000/webui-socket/" + session + "\x00",
	} {
		if got := searchSession([]byte(text)); got != session {
			t.Errorf("%q: got %q", text, got)
		}
	}
}

func TestSearchSessionRejectsShortLongAndCutOffDigits(t *testing.T) {
	for _, text := range []string{
		"/webui-socket/1234567890 ",            // 10 digits
		"/webui-socket/123456789012345678901 ", // 21 digits
		"/webui-socket/" + session,             // runs into the end of the buffer
	} {
		if got := searchSession([]byte(text)); got != "" {
			t.Errorf("%q: got %q", text, got)
		}
	}
	if got := searchSession([]byte("/webui-socket/1 then /webui-socket/" + session + "!")); got != session {
		t.Errorf("second match: got %q", got)
	}
}

func TestScanRegionsFindsASessionAcrossChunks(t *testing.T) {
	memory := make([]byte, 3*chunkSize)
	marker := []byte("/webui-socket/" + session + "\"")
	at := chunkSize - 20 // straddles the first chunk boundary
	copy(memory[at:], marker)
	read := func(buf []byte, addr uint64) (int, error) { return copy(buf, memory[addr:]), nil }
	if got := scanRegions(context.Background(), []region{{0, uint64(len(memory))}}, read); got != session {
		t.Fatalf("got %q", got)
	}
}

func TestScanRegionsSkipsUnreadableChunks(t *testing.T) {
	memory := make([]byte, 2*chunkSize)
	copy(memory[chunkSize+10:], "/webui-socket/"+session+"\"")
	read := func(buf []byte, addr uint64) (int, error) {
		if addr == 0 {
			return 0, errors.New("unreadable")
		}
		return copy(buf, memory[addr:]), nil
	}
	if got := scanRegions(context.Background(), []region{{0, uint64(len(memory))}}, read); got != session {
		t.Fatalf("got %q", got)
	}
}

func TestFixedFileVersion(t *testing.T) {
	data := make([]byte, 64)
	copy(data[10:], vsFixedFileInfoSignature)
	binary.LittleEndian.PutUint32(data[18:], 3<<16)
	binary.LittleEndian.PutUint32(data[22:], 1<<16|24371)
	if got := fixedFileVersion(data); got != "3.0.1.24371" {
		t.Fatalf("got %q", got)
	}
	if fixedFileVersion([]byte("nothing here")) != "" {
		t.Fatal("found a version in random data")
	}
}

func TestGameArgUnderWineAndProton(t *testing.T) {
	cases := map[string]bool{
		`C:\Program Files (x86)\Warcraft III\_retail_\x86_64\Warcraft III.exe`:             true,
		`Z:\home\me\Games\Warcraft III.exe`:                                                true,
		"wine64-preloader\x00C:\\Games\\Warcraft III.exe":                                  true,
		"wine\x00warcraft iii.exe":                                                         true,
		`C:\Program Files (x86)\Battle.net\Battle.net.exe`:                                 false,
		"start.exe\x00/unix\x00C:\\Games\\Warcraft III.exe":                                false,
		"/run/rosetta/rosetta\x00/usr/bin/wine64-preloader\x00C:\\Games\\Warcraft III.exe": true,
		"box64\x00wine64\x00C:\\Games\\Warcraft III.exe":                                   true,
		"/usr/bin/python3\x00C:\\Games\\Warcraft III.exe":                                  false,
		"bash\x00-c\x00wine 'C:\\Games\\Warcraft III.exe'":                                 false,
	}
	for cmd, want := range cases {
		if _, got := gameArg(strings.Split(cmd, "\x00")); got != want {
			t.Errorf("%q: got %v", cmd, got)
		}
	}
}

func TestDosPath(t *testing.T) {
	got, ok := dosPath("/pfx", `C:\Program Files (x86)\Warcraft III\Warcraft III.exe`)
	if !ok || got != "/pfx/dosdevices/c:/Program Files (x86)/Warcraft III/Warcraft III.exe" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := dosPath("/pfx", "relative.exe"); ok {
		t.Fatal("mapped a relative path")
	}
}

// fakeProc builds a /proc with one Wine game process (pid 4242) whose menu
// socket wineserver (pid 99) owns, plus an unrelated process.
func fakeProc(t *testing.T) (procfs, string) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("4242/cmdline", "C:\\Program Files (x86)\\Warcraft III\\_retail_\\x86_64\\Warcraft III.exe\x00-launch\x00")
	write("4242/status", "Name:\tWarcraft III.ex\nUid:\t1000\t1000\t1000\t1000\n")
	write("4242/environ", "HOME=/home/me\x00STEAM_COMPAT_DATA_PATH=/steam/compatdata/123\x00")
	write("4242/net/tcp", strings.Join([]string{
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
		"   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 5555 1",
		"   1: 0100007F:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 5556 1",
		"   2: 0100007F:2328 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 5557 1",
		"   3: 00000000:2710 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 5558 1",
		"   4: 0100007F:2711 0100007F:9999 01 00000000:00000000 00:00000000 00000000  1000        0 5559 1",
		"   5: 0100007F:C350 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 7777 1",
	}, "\n"))
	write("4242/maps", strings.Join([]string{
		"00400000-00401000 r--p 00000000 08:01 12 /steam/compatdata/123/pfx/drive_c/Program Files (x86)/Warcraft III/_retail_/x86_64/Warcraft III.exe",
		"10000-12000 rw-p 00000000 00:00 0 ",
		"20000-21000 r--p 00000000 00:00 0 ",
		"30000-31000 rw-s 00000000 00:01 9 /dev/shm/x",
		"40000-41000 rw-p 00000000 00:00 0 [heap]",
		"50000-51000 rw-p 00000000 08:01 13 /usr/lib/libc.so",
	}, "\n"))
	if err := os.MkdirAll(filepath.Join(root, "4242", "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("socket:[7777]", filepath.Join(root, "4242", "fd", "12")); err != nil {
		t.Fatal(err)
	}
	write("100/cmdline", "C:\\windows\\system32\\services.exe\x00")
	write("self/cmdline", "ignored")
	return procfs{root: root, home: "/home/me"}, root
}

func TestProcfsFindsTheWineGame(t *testing.T) {
	p, _ := fakeProc(t)
	g, err := p.findGame(context.Background())
	if err != nil || g.pid != 4242 || g.uid != 1000 {
		t.Fatalf("got %+v, %v", g, err)
	}
}

func TestProcfsRejectsTwoGames(t *testing.T) {
	p, root := fakeProc(t)
	_ = os.MkdirAll(filepath.Join(root, "5000"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "5000", "cmdline"), []byte("wine\x00Warcraft III.exe\x00"), 0o644)
	if _, err := p.findGame(context.Background()); !errors.Is(err, ErrManyGames) {
		t.Fatalf("err = %v", err)
	}
}

func TestProcfsListsOwnListenersFirstThenTheUsers(t *testing.T) {
	p, _ := fakeProc(t)
	ports, err := p.listeners(context.Background(), game{pid: 4242, uid: 1000})
	// 50000 is the game's socket; 8080 is the user's; 80 is privileged,
	// 9000 another user's, 10000 not loopback, 10001 not listening.
	if err != nil || !slices.Equal(ports, []int{50000, 8080}) {
		t.Fatalf("got %v, %v", ports, err)
	}
}

func TestProcfsPrivateRegionsAreAnonymousReadWrite(t *testing.T) {
	p, _ := fakeProc(t)
	regions, err := p.privateRegions(4242)
	want := []region{{0x10000, 0x12000}, {0x40000, 0x41000}}
	if err != nil || !slices.Equal(regions, want) {
		t.Fatalf("got %v, %v", regions, err)
	}
}

func TestProcfsReadsTheSessionFromMem(t *testing.T) {
	p, root := fakeProc(t)
	mem := make([]byte, 0x12000)
	copy(mem[0x10100:], "/webui/index.html?guid="+session+"&x")
	if err := os.WriteFile(filepath.Join(root, "4242", "mem"), mem, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := p.session(context.Background(), game{pid: 4242})
	if err != nil || got != session {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestProcfsReportsBlockedMemoryAccess(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs Unix file permissions, which root and Windows don't enforce")
	}
	p, root := fakeProc(t)
	mem := filepath.Join(root, "4242", "mem")
	_ = os.WriteFile(mem, nil, 0o000)
	if _, err := p.session(context.Background(), game{pid: 4242}); !errors.Is(err, ErrAccess) || !strings.Contains(err.Error(), "setcap") {
		t.Fatalf("err = %v", err)
	}
}

func TestProcfsExeCandidatesCoverProtonAndThePrefix(t *testing.T) {
	p, root := fakeProc(t)
	got := p.exeCandidates(4242)
	inRoot := path.Join(root, "4242", "root")
	mapped := "/steam/compatdata/123/pfx/drive_c/Program Files (x86)/Warcraft III/_retail_/x86_64/Warcraft III.exe"
	viaPrefix := "/steam/compatdata/123/pfx/dosdevices/c:/Program Files (x86)/Warcraft III/_retail_/x86_64/Warcraft III.exe"
	viaHome := "/home/me/.wine/dosdevices/c:/Program Files (x86)/Warcraft III/_retail_/x86_64/Warcraft III.exe"
	for _, want := range []string{inRoot + mapped, mapped, inRoot + viaPrefix, viaPrefix, inRoot + viaHome} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}

func TestParseLsof(t *testing.T) {
	out := "p10\ncWarcraft III\nn127.0.0.1:50000\nn127.0.0.1:80\np11\ncBlizzardBrowser\nn127.0.0.1:50001\np12\ncsomething\nn127.0.0.1:6000\n"
	if got := parseLsof([]byte(out), 10); !slices.Equal(got, []int{50000, 50001}) {
		t.Fatalf("got %v", got)
	}
}

type fakePlatform struct {
	ports     []int
	sessionID string
}

func (f fakePlatform) findGame(context.Context) (game, error)         { return game{pid: 7}, nil }
func (f fakePlatform) listeners(context.Context, game) ([]int, error) { return f.ports, nil }
func (f fakePlatform) session(context.Context, game) (string, error)  { return f.sessionID, nil }
func (f fakePlatform) version(context.Context, game) string           { return "3.0.1.24371" }

func TestDiscoverProbesForTheMenuPage(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello") }))
	defer other.Close()
	menu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/webui/index.html" {
			_, _ = io.WriteString(w, `<script src="GlueManager.js"></script>`)
		}
	}))
	defer menu.Close()
	port := func(url string) int { p, _ := strconv.Atoi(url[strings.LastIndex(url, ":")+1:]); return p }

	d := newDiscovery(fakePlatform{ports: []int{port(other.URL), port(menu.URL)}, sessionID: session}, slog.New(slog.DiscardHandler))
	conn, err := d.Discover(context.Background())
	if err != nil || conn.Port != port(menu.URL) || conn.Session != session || conn.Version != "3.0.1.24371" || conn.PID != 7 {
		t.Fatalf("got %+v, %v", conn, err)
	}

	d = newDiscovery(fakePlatform{ports: []int{port(other.URL)}, sessionID: session}, slog.New(slog.DiscardHandler))
	if _, err := d.Discover(context.Background()); !errors.Is(err, ErrNoMenu) {
		t.Fatalf("err = %v", err)
	}
	d = newDiscovery(fakePlatform{ports: []int{port(menu.URL)}, sessionID: "abc"}, slog.New(slog.DiscardHandler))
	if _, err := d.Discover(context.Background()); !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v", err)
	}
}
