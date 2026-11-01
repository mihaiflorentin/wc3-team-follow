package gui_test

import (
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/cmd/gui"
	mockcompanion "github.com/mihaiflorentin/wc3-team-follow/mock/companion"
)

type fakeWindow struct {
	frame   gui.Frame
	min     [2]int
	onTop   bool
	changes int
}

func (w *fakeWindow) Frame() gui.Frame        { return w.frame }
func (w *fakeWindow) SetFrame(f gui.Frame)    { w.frame = f; w.changes++ }
func (w *fakeWindow) SetMinSize(width, h int) { w.min = [2]int{width, h} }
func (w *fakeWindow) SetAlwaysOnTop(on bool)  { w.onTop = on }

func TestOverlayShrinksTheWindowOnTopAndPutsItBack(t *testing.T) {
	b := gui.NewBridge(&mockcompanion.Companion{}, "test", "", "")
	w := &fakeWindow{frame: gui.NewFrame(100, 50, 980, 760)}
	b.SetWindow(w, 720, 560)

	if err := b.SetOverlay(true); err != nil {
		t.Fatal(err)
	}
	if !w.onTop || w.frame != gui.NewFrame(780, 50, 300, 136) || w.min != [2]int{300, 136} {
		t.Fatalf("overlay: %+v", w)
	}
	// Asking again changes nothing.
	_ = b.SetOverlay(true)
	if w.changes != 1 {
		t.Fatalf("changed %d times", w.changes)
	}
	if err := b.SetOverlay(false); err != nil {
		t.Fatal(err)
	}
	if w.onTop || w.frame != gui.NewFrame(100, 50, 980, 760) || w.min != [2]int{720, 560} {
		t.Fatalf("back: %+v", w)
	}
}

func TestOverlayNeedsTheApp(t *testing.T) {
	b := gui.NewBridge(&mockcompanion.Companion{}, "test", "", "")
	if err := b.SetOverlay(true); err == nil {
		t.Fatal("no error outside the app")
	}
}
