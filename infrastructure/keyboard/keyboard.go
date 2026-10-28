// Package keyboard types one chat line into Warcraft III's match chat with
// keystrokes (docs/adr/0007-typing-in-the-match-chat-with-keystrokes.md).
//
// The sequence and its safety checks live here, in an untagged file, so
// they are tested on every OS. Each platform_<os>.go file supplies only the
// system calls: whether typing is possible, which process is frontmost, and
// pressing one key. Nothing here brings the game forward, reads or writes
// game memory, or sends anything but Enter and printable ASCII characters.
package keyboard

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// maxLine matches the game chat's own limit (follow.MaxChatLine).
const maxLine = 100

// ErrNotFrontmost reports that Warcraft III was not the frontmost window
// when a key was due, so that key and every key after it were not sent.
var ErrNotFrontmost = errors.New("Warcraft III isn't the window in front.")

// key is one key press: Enter, or one printable ASCII character.
type key struct {
	enter bool
	char  rune
}

// system is the OS seam. Every method is a plain system call with no
// sequencing of its own.
type system interface {
	// status reports whether typing is possible; it never prompts.
	status() dto.KeyboardStatus
	// requestPermission asks for the system permission typing needs.
	requestPermission() error
	// check refuses, before any key, a line this system can't type.
	check(line string) error
	// frontmostPID returns the process that receives key presses now,
	// or 0 when none does.
	frontmostPID() (int, error)
	// press sends one key down and up to the frontmost window.
	press(k key) error
}

// timing holds the pauses of the sequence.
type timing struct {
	// chatOpen is the wait after the first Enter for the chat box to open.
	chatOpen time.Duration
	// perKey is the wait after each character.
	perKey time.Duration
}

// defaultTiming follows the macOS live check (issue 9): about a second for
// the chat box, then one character at a time.
var defaultTiming = timing{chatOpen: time.Second, perKey: 40 * time.Millisecond}

// Keyboard is the contract.GameKeyboard of this package.
type Keyboard struct {
	sys    system
	timing timing
	sleep  func(context.Context, time.Duration) error
}

// New returns the keystroke input for this operating system.
func New() contract.GameKeyboard { return newKeyboard(newSystem(), defaultTiming) }

func newKeyboard(sys system, t timing) *Keyboard {
	return &Keyboard{sys: sys, timing: t, sleep: sleep}
}

// Status reports whether this build can type on this computer.
func (k *Keyboard) Status(context.Context) dto.KeyboardStatus { return k.sys.status() }

// RequestPermission asks the system for the permission typing needs.
func (k *Keyboard) RequestPermission(context.Context) error { return k.sys.requestPermission() }

// TypeLine presses Enter, waits for the chat box, types line one character
// at a time and presses Enter. Before every key it checks that pid is the
// frontmost process and stops the moment it isn't.
func (k *Keyboard) TypeLine(ctx context.Context, pid int, line string) error {
	if st := k.sys.status(); !st.Supported || !st.Permitted {
		return errors.New(st.Reason)
	}
	if pid <= 0 {
		return errors.New("Warcraft III isn't running.")
	}
	if err := checkLine(line); err != nil {
		return err
	}
	if err := k.sys.check(line); err != nil {
		return err
	}

	keys := make([]key, 0, len(line)+2)
	keys = append(keys, key{enter: true})
	for _, r := range line {
		keys = append(keys, key{char: r})
	}
	keys = append(keys, key{enter: true})

	for i, next := range keys {
		if err := ctx.Err(); err != nil {
			return stopped(i, err)
		}
		front, err := k.sys.frontmostPID()
		if err != nil {
			return stopped(i, err)
		}
		if front != pid {
			return stopped(i, ErrNotFrontmost)
		}
		if err := k.sys.press(next); err != nil {
			return stopped(i, err)
		}
		if i == len(keys)-1 {
			break
		}
		wait := k.timing.perKey
		if i == 0 {
			wait = k.timing.chatOpen
		}
		if err := k.sleep(ctx, wait); err != nil {
			return stopped(i+1, err)
		}
	}
	return nil
}

// stopped explains why typing stopped after sent keys. Once a key went
// out, the chat box may hold part of the line, so the player is told.
func stopped(sent int, err error) error {
	if sent == 0 {
		return err
	}
	return fmt.Errorf("Typing stopped after %d keys, so the game's chat box may hold part of the line: %w", sent, err)
}

// checkLine accepts one line of 1 to 100 printable ASCII characters: every
// known map command, without dead keys, input methods or layout surprises.
func checkLine(line string) error {
	if line == "" || utf8.RuneCountInString(line) > maxLine {
		return fmt.Errorf("The chat line must have 1 to %d characters.", maxLine)
	}
	for _, r := range line {
		if r < 0x20 || r > 0x7e {
			return errors.New("Team Follow types only letters, digits, spaces and common symbols in the match chat.")
		}
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
