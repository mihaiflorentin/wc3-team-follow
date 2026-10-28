//go:build darwin

package keyboard

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreGraphics -framework AppKit -framework ApplicationServices
#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>

static int wf_preflight(void) { return CGPreflightPostEventAccess() ? 1 : 0; }
static int wf_request(void) { return CGRequestPostEventAccess() ? 1 : 0; }

// wf_front_app is the pid of the app that receives key events.
static int wf_front_app(void) {
	@autoreleasepool {
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		return app == nil ? 0 : (int)app.processIdentifier;
	}
}

// wf_front_window is the owner of the topmost normal (layer 0) window on
// screen. NSWorkspace updates frontmostApplication from the main run loop,
// which a terminal command doesn't run, so this fresh answer is checked
// too. Owner pids and layers need no Screen Recording grant.
static int wf_front_window(void) {
	int pid = 0;
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (list == NULL) {
		return 0;
	}
	for (CFIndex i = 0; i < CFArrayGetCount(list); i++) {
		CFDictionaryRef w = CFArrayGetValueAtIndex(list, i);
		int layer = -1;
		CFNumberRef n = CFDictionaryGetValue(w, kCGWindowLayer);
		if (n == NULL || !CFNumberGetValue(n, kCFNumberIntType, &layer) || layer != 0) {
			continue;
		}
		CFNumberRef owner = CFDictionaryGetValue(w, kCGWindowOwnerPID);
		if (owner != NULL) {
			CFNumberGetValue(owner, kCFNumberIntType, &pid);
		}
		break;
	}
	CFRelease(list);
	return pid;
}

// wf_press posts one key down and up at the HID tap. ch is the character
// (0 for Return), code its virtual key code, shift whether it needs Shift.
static int wf_press(UniChar ch, CGKeyCode code, int shift) {
	CGEventFlags flags = shift ? kCGEventFlagMaskShift : 0;
	for (int down = 1; down >= 0; down--) {
		CGEventRef ev = CGEventCreateKeyboardEvent(NULL, code, down ? true : false);
		if (ev == NULL) {
			return 0;
		}
		CGEventSetFlags(ev, flags);
		if (ch != 0) {
			CGEventKeyboardSetUnicodeString(ev, 1, &ch);
		}
		CGEventPost(kCGHIDEventTap, ev);
		CFRelease(ev);
	}
	return 1;
}
*/
import "C"

import (
	"errors"
	"fmt"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	permissionReason = "Team Follow needs permission to send keystrokes. Allow it in System Settings, Privacy & Security, Accessibility, then try again."
	keyCodeReturn    = 36 // kVK_Return
)

// darwinSystem types with CGEventPost. It needs the event-synthesizing
// grant, which macOS ties to one exact build of an ad-hoc-signed app, so
// it is asked for again after every update.
type darwinSystem struct{}

func newSystem() system { return darwinSystem{} }

func (darwinSystem) status() dto.KeyboardStatus {
	if C.wf_preflight() == 0 {
		return dto.KeyboardStatus{Supported: true, Reason: permissionReason}
	}
	return dto.KeyboardStatus{Supported: true, Permitted: true}
}

func (darwinSystem) requestPermission() error {
	if C.wf_request() == 0 {
		return errors.New(permissionReason)
	}
	return nil
}

// check refuses a line with a character missing from usKeys before any
// key is pressed, so a line is never cut short.
func (darwinSystem) check(line string) error {
	for _, r := range line {
		if _, ok := usKeys[r]; !ok {
			return fmt.Errorf("Team Follow can't type %q in the match chat.", r)
		}
	}
	return nil
}

// frontmostPID returns the game's pid only when the frontmost app and the
// owner of the topmost window agree; otherwise 0, so typing stops.
func (darwinSystem) frontmostPID() (int, error) {
	app := int(C.wf_front_app())
	if win := int(C.wf_front_window()); win != app {
		return 0, nil
	}
	return app, nil
}

// press sends the character as its Unicode string together with its key
// code on a US layout, because Apple notes that some apps read the key
// code instead of the string.
func (darwinSystem) press(k key) error {
	var ch C.UniChar
	code, shift := uint16(keyCodeReturn), false
	if !k.enter {
		u, ok := usKeys[k.char]
		if !ok {
			return fmt.Errorf("Team Follow can't type %q in the match chat.", k.char)
		}
		ch, code, shift = C.UniChar(k.char), u.code, u.shift
	}
	s := C.int(0)
	if shift {
		s = 1
	}
	if C.wf_press(ch, C.CGKeyCode(code), s) == 0 {
		return errors.New("macOS refused the keystroke.")
	}
	return nil
}
