//go:build windows

package keyboard

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	inputKeyboard     = 1      // INPUT_KEYBOARD
	keyEventKeyUp     = 0x0002 // KEYEVENTF_KEYUP
	keyEventUnicode   = 0x0004 // KEYEVENTF_UNICODE
	virtualKeyReturn  = 0x0D   // VK_RETURN
	scanCodeReturn    = 0x1C   // Enter's set-1 scan code
	keyboardInputSize = 40     // sizeof(INPUT) on 64-bit Windows
)

var procSendInput = windows.NewLazySystemDLL("user32.dll").NewProc("SendInput")

// keyboardInput is INPUT with its KEYBDINPUT member, laid out by hand for
// 64-bit Windows: type, padding, then the union at offset 8, padded to the
// size of its largest member (MOUSEINPUT).
type keyboardInput struct {
	kind  uint32
	_     uint32
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	_     uint32
	extra uintptr
	_     [8]byte
}

// The build fails if the layout ever stops matching INPUT's size.
var _ = [1]struct{}{}[unsafe.Sizeof(keyboardInput{})-keyboardInputSize]

// windowsSystem types with SendInput. It needs no permission. Each key is
// one call (down and up together), so the player's own keys can't land
// inside a key press, and the foreground check runs before every call.
type windowsSystem struct{}

func newSystem() system { return windowsSystem{} }

func (windowsSystem) status() dto.KeyboardStatus {
	return dto.KeyboardStatus{Supported: true, Permitted: true}
}

func (windowsSystem) requestPermission() error { return nil }

// check accepts every line checkLine accepted: KEYEVENTF_UNICODE sends the
// character itself, whatever the keyboard layout.
func (windowsSystem) check(string) error { return nil }

// frontmostPID returns the process owning the foreground window. A NULL
// foreground window (focus changing) reports 0, which is never the game.
func (windowsSystem) frontmostPID() (int, error) {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return 0, nil
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil {
		return 0, fmt.Errorf("Team Follow couldn't tell which window is in front: %w", err)
	}
	return int(pid), nil
}

func (windowsSystem) press(k key) error {
	down := keyboardInput{kind: inputKeyboard}
	if k.enter {
		down.vk = virtualKeyReturn
		down.scan = scanCodeReturn
	} else {
		down.scan = uint16(k.char)
		down.flags = keyEventUnicode
	}
	up := down
	up.flags |= keyEventKeyUp
	inputs := [2]keyboardInput{down, up}
	n, _, err := procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), uintptr(keyboardInputSize))
	if int(n) != len(inputs) {
		return fmt.Errorf("Windows refused the keystroke: %w", err)
	}
	return nil
}
