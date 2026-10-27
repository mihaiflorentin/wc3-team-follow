package discovery

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// signal is one fake signal sent while closing games.
type signal struct {
	pid   int
	force bool
}

// fakeSignals records signals instead of sending them. A pid in quits
// exits on the first request (its /proc entry disappears); others ignore
// it and exit only when forced.
type fakeSignals struct {
	mu    sync.Mutex
	root  string
	quits map[int]bool
	sent  []signal
}

func (f *fakeSignals) send(pid int, force bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, signal{pid, force})
	if force || f.quits[pid] {
		return os.RemoveAll(filepath.Join(f.root, strconv.Itoa(pid)))
	}
	return nil
}

// addGame adds a Wine game process to the fake /proc; uid < 0 leaves out
// its status file, so its owner is unknown.
func addGame(t *testing.T, root string, pid, uid int) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte("wine64-preloader\x00C:\\Games\\Warcraft III.exe\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if uid >= 0 {
		status := "Name:\tWarcraft III.ex\nUid:\t" + strconv.Itoa(uid) + "\t0\t0\t0\n"
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCloseAllAsksThenForcesOnlyThisUsersGames(t *testing.T) {
	p, root := fakeProc(t)       // 4242 is this user's game (uid 1000), 100 is services.exe
	addGame(t, root, 5000, 1000) // this user's, ignores the request
	addGame(t, root, 6000, 2000) // another user's
	addGame(t, root, 7000, -1)   // owner unknown
	signals := &fakeSignals{root: root, quits: map[int]bool{4242: true}}
	p.signal = signals.send
	closer := newProcesses(p, slog.New(slog.DiscardHandler))
	closer.grace, closer.poll = 200*time.Millisecond, 5*time.Millisecond

	n, err := closer.CloseAll(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("CloseAll = %d, %v", n, err)
	}
	want := []signal{{4242, false}, {5000, false}, {5000, true}}
	if !slices.Equal(signals.sent, want) {
		t.Fatalf("signals = %v, want %v", signals.sent, want)
	}
	games, _ := p.findGames(context.Background())
	var left []int
	for _, g := range games {
		left = append(left, g.pid)
	}
	slices.Sort(left)
	if !slices.Equal(left, []int{6000, 7000}) {
		t.Fatalf("games left = %v", left)
	}
}

func TestCloseAllWithNoGameRunningSignalsNothing(t *testing.T) {
	p, root := fakeProc(t)
	if err := os.RemoveAll(filepath.Join(root, "4242")); err != nil {
		t.Fatal(err)
	}
	signals := &fakeSignals{root: root}
	p.signal = signals.send
	n, err := newProcesses(p, slog.New(slog.DiscardHandler)).CloseAll(context.Background())
	if err != nil || n != 0 || len(signals.sent) != 0 {
		t.Fatalf("CloseAll = %d, %v, signals %v", n, err, signals.sent)
	}
}

func TestStopLeavesAReusedPidAlone(t *testing.T) {
	p, root := fakeProc(t)
	signals := &fakeSignals{root: root}
	p.signal = signals.send
	// pid 100 runs services.exe, not the game.
	if err := p.stop(game{pid: 100, uid: 1000}, true); err != nil || len(signals.sent) != 0 {
		t.Fatalf("stop = %v, signals %v", err, signals.sent)
	}
}
