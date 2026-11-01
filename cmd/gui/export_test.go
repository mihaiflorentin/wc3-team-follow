package gui

// SetOpenURL replaces how the bridge opens a web page, for tests.
func (b *Bridge) SetOpenURL(fn func(link string) error) { b.openURL = fn }

// Frame is the window's position and size, for tests.
type Frame = frame

// SetWindow replaces the window the overlay changes, for tests.
func (b *Bridge) SetWindow(w interface {
	Frame() Frame
	SetFrame(f Frame)
	SetMinSize(width, height int)
	SetAlwaysOnTop(on bool)
}, minWidth, minHeight int) {
	b.window = w
	b.overlay.minSize = [2]int{minWidth, minHeight}
}

// NewFrame builds a Frame, for tests.
func NewFrame(x, y, width, height int) Frame { return frame{x, y, width, height} }
