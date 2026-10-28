package keyboard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const gamePID = 4242

// fakeOS records every key and every pause in one timeline, and answers
// the frontmost check from front (one answer per check, then the last
// answer repeats).
type fakeOS struct {
	st       dto.KeyboardStatus
	permErr  error
	front    []int
	checks   int
	timeline []string
	pressErr error
	refuse   error
}

func (f *fakeOS) status() dto.KeyboardStatus { return f.st }
func (f *fakeOS) requestPermission() error   { return f.permErr }
func (f *fakeOS) check(string) error         { return f.refuse }

func (f *fakeOS) frontmostPID() (int, error) {
	i := min(f.checks, len(f.front)-1)
	f.checks++
	f.timeline = append(f.timeline, "check")
	return f.front[i], nil
}

func (f *fakeOS) press(k key) error {
	if k.enter {
		f.timeline = append(f.timeline, "Enter")
	} else {
		f.timeline = append(f.timeline, string(k.char))
	}
	return f.pressErr
}

func ready() *fakeOS {
	return &fakeOS{st: dto.KeyboardStatus{Supported: true, Permitted: true}, front: []int{gamePID}}
}

func newTest(f *fakeOS) *Keyboard {
	k := newKeyboard(f, timing{chatOpen: time.Second, perKey: 40 * time.Millisecond})
	k.sleep = func(_ context.Context, d time.Duration) error {
		f.timeline = append(f.timeline, "wait "+d.String())
		return nil
	}
	return k
}

func TestTypeLineOpensChatWaitsTypesEachKeyAndSends(t *testing.T) {
	f := ready()
	if err := newTest(f).TypeLine(context.Background(), gamePID, "-ar"); err != nil {
		t.Fatal(err)
	}
	want := "check Enter wait 1s check - wait 40ms check a wait 40ms check r wait 40ms check Enter"
	if got := strings.Join(f.timeline, " "); got != want {
		t.Fatalf("timeline\n got: %s\nwant: %s", got, want)
	}
}

func TestTypeLineTypesNothingWhenTheGameIsNotFrontmost(t *testing.T) {
	f := ready()
	f.front = []int{99}
	err := newTest(f).TypeLine(context.Background(), gamePID, "-ar")
	if !errors.Is(err, ErrNotFrontmost) {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(f.timeline, " "); got != "check" {
		t.Fatalf("timeline = %s", got)
	}
}

func TestTypeLineStopsTheMomentFocusLeaves(t *testing.T) {
	f := ready()
	f.front = []int{gamePID, gamePID, gamePID, 0}
	err := newTest(f).TypeLine(context.Background(), gamePID, "-ar")
	if !errors.Is(err, ErrNotFrontmost) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "part of the line") {
		t.Fatalf("err should warn about a half-typed line: %v", err)
	}
	if got := strings.Join(f.timeline, " "); got != "check Enter wait 1s check - wait 40ms check a wait 40ms check" {
		t.Fatalf("timeline = %s", got)
	}
}

func TestTypeLineStopsWhenCancelled(t *testing.T) {
	f := ready()
	ctx, cancel := context.WithCancel(context.Background())
	k := newTest(f)
	k.sleep = func(ctx context.Context, d time.Duration) error {
		f.timeline = append(f.timeline, "wait "+d.String())
		cancel()
		return ctx.Err()
	}
	err := k.TypeLine(ctx, gamePID, "-ar")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(f.timeline, " "); got != "check Enter wait 1s" {
		t.Fatalf("timeline = %s", got)
	}
}

func TestTypeLineRefusesBeforeAnyKey(t *testing.T) {
	cases := map[string]func(*fakeOS) (int, string){
		"unsupported": func(f *fakeOS) (int, string) {
			f.st = dto.KeyboardStatus{Reason: "Typing in the match chat isn't supported on Linux yet."}
			return gamePID, "-ar"
		},
		"not permitted": func(f *fakeOS) (int, string) {
			f.st = dto.KeyboardStatus{Supported: true, Reason: "Allow it."}
			return gamePID, "-ar"
		},
		"no game":       func(*fakeOS) (int, string) { return 0, "-ar" },
		"empty":         func(*fakeOS) (int, string) { return gamePID, "" },
		"too long":      func(*fakeOS) (int, string) { return gamePID, strings.Repeat("a", 101) },
		"not ascii":     func(*fakeOS) (int, string) { return gamePID, "héllo" },
		"control":       func(*fakeOS) (int, string) { return gamePID, "a\nb" },
		"layout refuse": func(f *fakeOS) (int, string) { f.refuse = errors.New("No."); return gamePID, "-ar" },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := ready()
			pid, line := setup(f)
			err := newTest(f).TypeLine(context.Background(), pid, line)
			if err == nil {
				t.Fatal("no error")
			}
			if len(f.timeline) != 0 {
				t.Fatalf("timeline = %v", f.timeline)
			}
			if msg := err.Error(); msg[0] < 'A' || msg[0] > 'Z' || !strings.HasSuffix(msg, ".") {
				t.Fatalf("not a sentence: %q", msg)
			}
		})
	}
}

func TestTypeLineReportsAFailedKey(t *testing.T) {
	f := ready()
	f.pressErr = errors.New("Windows refused the keystroke.")
	err := newTest(f).TypeLine(context.Background(), gamePID, "-ar")
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(f.timeline, " "); got != "check Enter" {
		t.Fatalf("timeline = %s", got)
	}
}

func TestUSKeysCoverEveryPrintableASCIICharacter(t *testing.T) {
	for r := rune(0x20); r <= 0x7e; r++ {
		if _, ok := usKeys[r]; !ok {
			t.Errorf("%q has no key", r)
		}
	}
	// Known codes from HIToolbox's Events.h.
	want := map[rune]usKey{
		'a': {code: 0x00}, 'A': {code: 0x00, shift: true}, '-': {code: 0x1B},
		'!': {code: 0x12, shift: true}, '5': {code: 0x17}, ' ': {code: 0x31},
	}
	for r, k := range want {
		if usKeys[r] != k {
			t.Errorf("%q = %+v, want %+v", r, usKeys[r], k)
		}
	}
}

func TestStatusAndPermissionComeFromTheSystem(t *testing.T) {
	f := ready()
	f.st = dto.KeyboardStatus{Supported: true, Reason: "Allow it."}
	f.permErr = errors.New("Denied.")
	k := newTest(f)
	if got := k.Status(context.Background()); got != f.st {
		t.Fatalf("status = %+v", got)
	}
	if err := k.RequestPermission(context.Background()); !errors.Is(err, f.permErr) {
		t.Fatalf("err = %v", err)
	}
}
