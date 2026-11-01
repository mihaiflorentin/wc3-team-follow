package gui

import (
	"context"
	"errors"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The overlay's window size: a small panel kept on top of the game.
const (
	overlayWidth  = 300
	overlayHeight = 164
)

// frame is the window's position and size.
type frame struct{ x, y, width, height int }

// windowControl is what the overlay changes about the window: the Wails
// runtime in the app, a fake in tests.
type windowControl interface {
	Frame() frame
	SetFrame(f frame)
	SetMinSize(width, height int)
	SetAlwaysOnTop(on bool)
}

// overlayState remembers the window as it was before the overlay, so
// leaving the overlay puts it back.
type overlayState struct {
	on      bool
	before  frame
	minSize [2]int
}

// SetOverlay shrinks the window into a small panel that stays on top of
// other windows, the game included when it runs windowed, or puts it back
// as it was. It changes only this window.
func (b *Bridge) SetOverlay(on bool) error {
	w := b.window
	if w == nil {
		if b.ctx == nil {
			return errors.New("The overlay is only available in the app.")
		}
		w = wailsWindow{b.ctx}
	}
	if on == b.overlay.on {
		return nil
	}
	if on {
		f := w.Frame()
		b.overlay.before = f
		w.SetMinSize(overlayWidth, overlayHeight)
		// Keep the panel's top-right corner where the window's was.
		w.SetFrame(frame{x: f.x + f.width - overlayWidth, y: f.y, width: overlayWidth, height: overlayHeight})
		w.SetAlwaysOnTop(true)
	} else {
		w.SetAlwaysOnTop(false)
		w.SetMinSize(b.overlay.minSize[0], b.overlay.minSize[1])
		w.SetFrame(b.overlay.before)
	}
	b.overlay.on = on
	return nil
}

// wailsWindow changes the app's window through the Wails runtime.
type wailsWindow struct{ ctx context.Context }

func (w wailsWindow) Frame() frame {
	x, y := wailsruntime.WindowGetPosition(w.ctx)
	width, height := wailsruntime.WindowGetSize(w.ctx)
	return frame{x, y, width, height}
}

func (w wailsWindow) SetFrame(f frame) {
	wailsruntime.WindowSetSize(w.ctx, f.width, f.height)
	wailsruntime.WindowSetPosition(w.ctx, f.x, f.y)
}

func (w wailsWindow) SetMinSize(width, height int) {
	wailsruntime.WindowSetMinSize(w.ctx, width, height)
}

func (w wailsWindow) SetAlwaysOnTop(on bool) { wailsruntime.WindowSetAlwaysOnTop(w.ctx, on) }
