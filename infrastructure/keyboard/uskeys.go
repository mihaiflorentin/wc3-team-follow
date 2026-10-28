package keyboard

// usKey is the macOS virtual key code of a character on the US (ANSI)
// layout, and whether it needs Shift.
type usKey struct {
	code  uint16
	shift bool
}

// usKeys maps every printable ASCII character to its key on the US layout
// (kVK_ANSI_* in HIToolbox's Events.h). macOS posts the character itself
// as the event's Unicode string too; the key code is for apps that read
// the code instead. It is untagged so it is tested on every OS.
var usKeys = buildUSKeys()

func buildUSKeys() map[rune]usKey {
	plain := map[rune]uint16{
		'a': 0x00, 's': 0x01, 'd': 0x02, 'f': 0x03, 'h': 0x04, 'g': 0x05, 'z': 0x06, 'x': 0x07,
		'c': 0x08, 'v': 0x09, 'b': 0x0B, 'q': 0x0C, 'w': 0x0D, 'e': 0x0E, 'r': 0x0F, 'y': 0x10,
		't': 0x11, '1': 0x12, '2': 0x13, '3': 0x14, '4': 0x15, '6': 0x16, '5': 0x17, '=': 0x18,
		'9': 0x19, '7': 0x1A, '-': 0x1B, '8': 0x1C, '0': 0x1D, ']': 0x1E, 'o': 0x1F, 'u': 0x20,
		'[': 0x21, 'i': 0x22, 'p': 0x23, 'l': 0x25, 'j': 0x26, '\'': 0x27, 'k': 0x28, ';': 0x29,
		'\\': 0x2A, ',': 0x2B, '/': 0x2C, 'n': 0x2D, 'm': 0x2E, '.': 0x2F, ' ': 0x31, '`': 0x32,
	}
	shifted := map[rune]rune{
		'!': '1', '@': '2', '#': '3', '$': '4', '%': '5', '^': '6', '&': '7', '*': '8',
		'(': '9', ')': '0', '_': '-', '+': '=', '{': '[', '}': ']', '|': '\\', ':': ';',
		'"': '\'', '<': ',', '>': '.', '?': '/', '~': '`',
	}
	keys := make(map[rune]usKey, 95)
	for r, code := range plain {
		keys[r] = usKey{code: code}
		if r >= 'a' && r <= 'z' {
			keys[r-'a'+'A'] = usKey{code: code, shift: true}
		}
	}
	for r, base := range shifted {
		keys[r] = usKey{code: plain[base], shift: true}
	}
	return keys
}
